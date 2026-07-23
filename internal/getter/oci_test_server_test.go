package getter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// testMainTF is a shared filename constant for OCI test file maps.
// Centralised here to satisfy the goconst linter across the getter_test package.
const testMainTF = "main.tf"

// ociManifest is a minimal OCI Image Manifest for test use.
type ociManifest struct { //nolint:govet
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociDescriptor struct { //nolint:govet
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// ociTestServer is a minimal OCI Distribution Spec v2 server for tests.
// It serves a single artifact whose content is determined by the files map
// passed to newOCITestServer. All requests succeed without auth challenges.
type ociTestServer struct { //nolint:govet
	manifestGets   atomic.Int32
	blobGets       atomic.Int32
	manifestDigest string
	manifestBytes  []byte
	layerDigest    string
	layerBytes     []byte
}

// newOCITestServer builds an in-memory tar.gz layer from files, constructs
// a well-formed OCI Image Manifest, and starts a plain-HTTP httptest.Server
// that speaks the OCI Distribution Spec v2. The server is registered for
// t.Cleanup.
//
// httptest.NewServer binds to 127.0.0.1. OCIGetter and OCIResolver detect
// 127.0.0.1 in the registry address and set PlainHTTP=true, so no TLS
// configuration is needed in tests.
func newOCITestServer(t *testing.T, files map[string]string) (*httptest.Server, *ociTestServer) {
	t.Helper()

	state := &ociTestServer{}

	state.layerBytes = makeTarGz(t, files)
	state.layerDigest = "sha256:" + hexSHA256(state.layerBytes)

	configBytes := []byte("{}")
	configDigest := "sha256:" + hexSHA256(configBytes)

	m := ociManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		},
		Layers: []ociDescriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    state.layerDigest,
				Size:      int64(len(state.layerBytes)),
			},
		},
	}

	var err error

	state.manifestBytes, err = json.Marshal(m)
	require.NoError(t, err)

	state.manifestDigest = "sha256:" + hexSHA256(state.manifestBytes)

	srv := httptest.NewServer(state)
	t.Cleanup(srv.Close)

	return srv, state
}

// manifestRE matches /v2/<name>/manifests/<ref> where <name> may contain slashes.
var manifestRE = regexp.MustCompile(`^/v2/(.+)/manifests/(.+)$`)

// blobRE matches /v2/<name>/blobs/<digest>.
var blobRE = regexp.MustCompile(`^/v2/(.+)/blobs/(.+)$`)

func (s *ociTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// OCI API v2 ping — required by some oras-go code paths.
	if r.URL.Path == "/v2/" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{}")

		return
	}

	if manifestRE.MatchString(r.URL.Path) {
		s.manifestGets.Add(1)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", s.manifestDigest)
		w.Header().Set("Content-Length", strconv.Itoa(len(s.manifestBytes)))
		w.WriteHeader(http.StatusOK)

		if r.Method != http.MethodHead {
			_, _ = w.Write(s.manifestBytes)
		}

		return
	}

	if m := blobRE.FindStringSubmatch(r.URL.Path); m != nil {
		requestedDigest := m[2]

		if requestedDigest == s.layerDigest {
			s.blobGets.Add(1)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(len(s.layerBytes)))
			w.WriteHeader(http.StatusOK)

			if r.Method != http.MethodHead {
				_, _ = w.Write(s.layerBytes)
			}

			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
