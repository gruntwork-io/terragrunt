// Package ociregistry serves an in-process OCI Distribution registry for tests.
package ociregistry

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/pem"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
)

const (
	// ArtifactTypeModulePkg is OpenTofu's module-package artifact type, transcribed from the upstream spec.
	ArtifactTypeModulePkg = "application/vnd.opentofu.modulepkg"
	// MediaTypeModuleZip is the media type OpenTofu gives a module-package zip layer.
	MediaTypeModuleZip = "archive/zip"
)

const (
	// packCreated pins the packed manifest timestamp so a published module keeps one digest.
	packCreated = "2026-01-01T00:00:00Z"
	// headerContentDigest is the header oras reads a resolved digest from.
	headerContentDigest = "Docker-Content-Digest"
	// headerContentType names the media type of a response body.
	headerContentType = "Content-Type"
	// pathManifests is the distribution-spec path segment addressing manifests.
	pathManifests = "manifests"
	// pathBlobs is the distribution-spec path segment addressing blobs.
	pathBlobs = "blobs"
	// codeManifestUnknown is the distribution-spec error code for an absent manifest.
	codeManifestUnknown = "MANIFEST_UNKNOWN"
	// codeBlobUnknown is the distribution-spec error code for an absent blob.
	codeBlobUnknown = "BLOB_UNKNOWN"
	// codeNameUnknown is the distribution-spec error code for an absent repository.
	codeNameUnknown = "NAME_UNKNOWN"
)

// Registry is an in-process OCI Distribution registry serving module packages over TLS.
type Registry struct {
	server *httptest.Server
	repos  map[string]*repository
	mu     sync.RWMutex
}

// repository holds one repository's content, so a blob is unreachable through any other name.
type repository struct {
	manifests map[digest.Digest]storedBlob
	blobs     map[digest.Digest]storedBlob
	tags      map[string]digest.Digest
}

// storedBlob is one stored object and the media type the registry reports for it.
type storedBlob struct {
	mediaType string
	data      []byte
}

// Start returns a running registry the test tears down on cleanup.
func Start(t *testing.T) *Registry {
	t.Helper()

	r := &Registry{repos: map[string]*repository{}}
	r.server = httptest.NewTLSServer(http.HandlerFunc(r.serve))

	t.Cleanup(r.server.Close)

	return r
}

// Address returns the host:port to use as the oci:// registry domain.
func (r *Registry) Address() string {
	return r.server.Listener.Addr().String()
}

// Client returns an HTTP client that trusts the registry's certificate.
func (r *Registry) Client() *http.Client {
	return r.server.Client()
}

// CertPEM returns the registry's certificate, so a child process can trust it through SSL_CERT_FILE.
func (r *Registry) CertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.server.Certificate().Raw})
}

// PushModule packs files into a module zip, publishes it under repo and tag, and returns the manifest descriptor.
func (r *Registry) PushModule(t *testing.T, repo, tag string, files map[string]string) ociv1.Descriptor {
	t.Helper()

	staging := memory.New()
	zipBytes := moduleZip(t, files)

	layer := ociv1.Descriptor{
		MediaType: MediaTypeModuleZip,
		Digest:    digest.FromBytes(zipBytes),
		Size:      int64(len(zipBytes)),
	}
	require.NoError(t, staging.Push(t.Context(), layer, bytes.NewReader(zipBytes)))

	// Packing through oras builds the manifest the same way a publisher would.
	manifestDesc, err := oras.PackManifest(
		t.Context(),
		staging,
		oras.PackManifestVersion1_1,
		ArtifactTypeModulePkg,
		oras.PackManifestOptions{
			Layers:              []ociv1.Descriptor{layer},
			ManifestAnnotations: map[string]string{ociv1.AnnotationCreated: packCreated},
		},
	)
	require.NoError(t, err)

	manifestBytes := readDescriptor(t, staging, &manifestDesc)

	var manifest ociv1.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	r.mu.Lock()
	defer r.mu.Unlock()

	target := r.repositoryLocked(repo)
	target.manifests[manifestDesc.Digest] = storedBlob{mediaType: manifestDesc.MediaType, data: manifestBytes}
	target.tags[tag] = manifestDesc.Digest

	contents := append([]ociv1.Descriptor{manifest.Config}, manifest.Layers...)
	for i := range contents {
		desc := &contents[i]
		target.blobs[desc.Digest] = storedBlob{mediaType: desc.MediaType, data: readDescriptor(t, staging, desc)}
	}

	return manifestDesc
}

// OverwriteBlob replaces a blob's bytes while the registry keeps advertising its original digest.
func (r *Registry) OverwriteBlob(t *testing.T, repo string, target digest.Digest, data []byte) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	held, ok := r.repos[repo]
	require.True(t, ok, "repository %s was never pushed", repo)

	stored, ok := held.blobs[target]
	require.True(t, ok, "blob %s was never pushed to %s", target, repo)

	stored.data = data
	held.blobs[target] = stored
}

// serve routes the subset of the distribution API a module download exercises.
func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/v2" || req.URL.Path == "/v2/" {
		w.Header().Set(headerContentType, "application/json")
		writeBody(w, []byte("{}"))

		return
	}

	name, kind, ref, ok := splitAPIPath(req.URL.Path)
	if !ok {
		writeNotFound(w, codeNameUnknown)

		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	held, ok := r.repos[name]
	if !ok {
		writeNotFound(w, codeNameUnknown)

		return
	}

	if kind == pathBlobs {
		serveBlob(w, req, held, ref)

		return
	}

	serveManifest(w, req, held, ref)
}

// repositoryLocked returns repo's content, creating it on first push, and expects the write lock held.
func (r *Registry) repositoryLocked(repo string) *repository {
	held, ok := r.repos[repo]
	if ok {
		return held
	}

	created := &repository{
		manifests: map[digest.Digest]storedBlob{},
		blobs:     map[digest.Digest]storedBlob{},
		tags:      map[string]digest.Digest{},
	}
	r.repos[repo] = created

	return created
}

// serveManifest answers the HEAD that resolves a reference and the GET that reads the manifest.
func serveManifest(w http.ResponseWriter, req *http.Request, held *repository, ref string) {
	target, ok := resolveReference(held, ref)
	if !ok {
		writeNotFound(w, codeManifestUnknown)

		return
	}

	stored, ok := held.manifests[target]
	if !ok {
		writeNotFound(w, codeManifestUnknown)

		return
	}

	w.Header().Set(headerContentDigest, target.String())
	writeContent(w, req, stored)
}

// serveBlob answers the GET that streams a layer.
func serveBlob(w http.ResponseWriter, req *http.Request, held *repository, ref string) {
	target, err := digest.Parse(ref)
	if err != nil {
		writeNotFound(w, codeBlobUnknown)

		return
	}

	stored, ok := held.blobs[target]
	if !ok {
		writeNotFound(w, codeBlobUnknown)

		return
	}

	// The advertised digest stays the pushed one, so an overwritten blob reaches the caller's verifier.
	w.Header().Set(headerContentDigest, target.String())
	writeContent(w, req, stored)
}

// resolveReference turns a tag or a digest reference into the digest it names.
func resolveReference(held *repository, ref string) (digest.Digest, bool) {
	tagged, ok := held.tags[ref]
	if ok {
		return tagged, true
	}

	parsed, err := digest.Parse(ref)
	if err != nil {
		return "", false
	}

	return parsed, true
}

// writeContent sends stored with the length oras requires, omitting the body for a HEAD.
func writeContent(w http.ResponseWriter, req *http.Request, stored storedBlob) {
	w.Header().Set(headerContentType, stored.mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(stored.data)))

	if req.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)

		return
	}

	writeBody(w, stored.data)
}

// writeNotFound answers with the distribution-spec error body a registry client parses.
func writeNotFound(w http.ResponseWriter, code string) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(http.StatusNotFound)
	writeBody(w, []byte(`{"errors":[{"code":"`+code+`"}]}`))
}

// writeBody writes a response body, since a broken test connection is not the registry's concern.
func writeBody(w http.ResponseWriter, data []byte) {
	_, _ = w.Write(data)
}

// splitAPIPath splits /v2/<name>/<kind>/<ref>, keeping a multi-segment repository name whole.
func splitAPIPath(path string) (string, string, string, bool) {
	rest, ok := strings.CutPrefix(path, "/v2/")
	if !ok {
		return "", "", "", false
	}

	// Neither a tag nor a digest holds a slash, so the last two segments are always the kind and the reference.
	refAt := strings.LastIndex(rest, "/")
	if refAt < 0 {
		return "", "", "", false
	}

	kindAt := strings.LastIndex(rest[:refAt], "/")
	if kindAt < 0 {
		return "", "", "", false
	}

	kind := rest[kindAt+1 : refAt]
	if kind != pathManifests && kind != pathBlobs {
		return "", "", "", false
	}

	return rest[:kindAt], kind, rest[refAt+1:], true
}

// moduleZip builds the module package zip the registry serves as its single layer.
func moduleZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	archive := zip.NewWriter(&buf)

	// Sorted names keep one file set packing to one digest.
	for _, name := range slices.Sorted(maps.Keys(files)) {
		entry, err := archive.Create(name)
		require.NoError(t, err)

		_, err = io.WriteString(entry, files[name])
		require.NoError(t, err)
	}

	require.NoError(t, archive.Close())

	return buf.Bytes()
}

// readDescriptor reads one descriptor's bytes back out of the staging store.
func readDescriptor(t *testing.T, staging *memory.Store, desc *ociv1.Descriptor) []byte {
	t.Helper()

	reader, err := staging.Fetch(t.Context(), *desc)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reader.Close())
	}()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	return data
}
