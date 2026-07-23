package getter

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// ociMediaTypes is the ordered preference list of layer media types that contain
// module content. The first match in a manifest's layer list wins.
var ociMediaTypes = []string{
	"application/vnd.opentofu.modulepkg",
	"application/vnd.terraform.module.v1+tar",
	ocispec.MediaTypeImageLayerGzip,
	ocispec.MediaTypeImageLayer,
	"application/octet-stream",
}

// OCIGetter is the go-getter v2 implementation of the oci:// protocol.
//
// Source URLs take the form:
//
//	oci://REGISTRY/REPOSITORY[:<tag>|@<digest>][//<subdir>]
//
// Examples:
//
//	oci://ghcr.io/org/terraform-aws-vpc:v1.0.0
//	oci://123456789.dkr.ecr.us-east-1.amazonaws.com/modules/vpc:v1.0.0
//	oci://ghcr.io/org/modules:v1.0.0//vpc
//
// Authentication is resolved in order:
//  1. TG_OCI_TOKEN environment variable (Bearer token)
//  2. TG_OCI_USERNAME + TG_OCI_PASSWORD environment variables (Basic auth)
//  3. Docker config file (~/.docker/config.json) via the oras credential store,
//     which delegates to configured credential helpers (docker-credential-ecr-login,
//     docker-credential-osxkeychain, etc.)
//  4. Unauthenticated (public registries)
type OCIGetter struct {
	HTTPClient *http.Client
	Logger     log.Logger
	FS         vfs.FS
	// PlainHTTP forces plain HTTP for all requests to the registry. When false
	// (the default), HTTPS is used for all non-loopback registries; loopback
	// addresses (localhost, 127.0.0.1, ::1) use HTTP automatically to support
	// local development registries (kind, k3d, airgap mirrors).
	PlainHTTP bool
}

// NewOCIGetter returns an [OCIGetter] configured with sensible defaults.
func NewOCIGetter(l log.Logger, fs vfs.FS) *OCIGetter {
	return &OCIGetter{
		HTTPClient: http.DefaultClient,
		Logger:     l,
		FS:         fs,
	}
}

// Mode reports directory mode for all oci:// sources.
func (g *OCIGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect recognizes oci:// sources and oci-forced sources.
func (g *OCIGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == SchemeOCI {
		return true, nil
	}

	u, err := url.Parse(req.Src)
	if err == nil && u.Scheme == SchemeOCI {
		return true, nil
	}

	return false, nil
}

// Get fetches the OCI artifact at req.Src and extracts its module content into req.Dst.
func (g *OCIGetter) Get(ctx context.Context, req *getter.Request) error {
	// go-getter's Client strips //subdir from Src before calling Get; direct
	// callers (e.g. unit tests) may still carry it, so always strip here.
	baseSrc, subDir := SourceDirSubdir(req.Src)

	srcURL, err := url.Parse(baseSrc)
	if err != nil {
		return fmt.Errorf("oci: parsing source URL %q: %w", baseSrc, err)
	}

	ref := ociRefFromURL(srcURL)
	g.Logger.Debugf("OCI getter: fetching %s", ref)

	repo, err := g.newRepository(ref)
	if err != nil {
		return fmt.Errorf("oci: building repository client for %s: %w", ref, err)
	}

	descriptor, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return fmt.Errorf("oci: resolving %s: %w", ref, err)
	}

	layer, err := g.resolveLayer(ctx, repo, &descriptor)
	if err != nil {
		return err
	}

	g.Logger.Debugf("OCI getter: downloading layer %s (%s, %d bytes)", layer.Digest, layer.MediaType, layer.Size)

	rc, err := repo.Blobs().Fetch(ctx, layer)
	if err != nil {
		return fmt.Errorf("oci: fetching blob %s: %w", layer.Digest, err)
	}

	defer rc.Close() //nolint:errcheck

	if subDir == "" {
		return extractTarLayer(layer.MediaType, rc, req.Dst)
	}

	return g.extractSubdir(layer.MediaType, rc, req.Dst, subDir)
}

// GetFile is not supported for OCI sources.
func (g *OCIGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return errors.New("GetFile is not supported for the OCI getter")
}

// resolveLayer picks the best content layer from the manifest at desc.
// It handles OCI Image Manifests and falls back gracefully to the first layer.
func (g *OCIGetter) resolveLayer(
	ctx context.Context,
	repo *remote.Repository,
	desc *ocispec.Descriptor,
) (ocispec.Descriptor, error) {
	_, manifestBytes, err := oras.FetchBytes(ctx, repo, desc.Digest.String(), oras.DefaultFetchBytesOptions)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("oci: fetching manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("oci: parsing manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("oci: artifact at %s has no layers", desc.Digest)
	}

	for _, preferred := range ociMediaTypes {
		for _, layer := range manifest.Layers {
			if layer.MediaType == preferred {
				return layer, nil
			}
		}
	}

	return manifest.Layers[0], nil
}

// extractSubdir extracts rc into a temp dir then copies out the requested subdirectory.
func (g *OCIGetter) extractSubdir(mediaType string, rc io.ReadCloser, dst, subDir string) error {
	tmpParent, err := vfs.MkdirTemp(g.FS, "", "oci-getter")
	if err != nil {
		return err
	}

	defer func() {
		if err := g.FS.RemoveAll(tmpParent); err != nil {
			g.Logger.Warnf("OCI getter: failed to remove temp dir %s: %v", tmpParent, err)
		}
	}()

	tmpDir := filepath.Join(tmpParent, "extract")

	if err := extractTarLayer(mediaType, rc, tmpDir); err != nil {
		return err
	}

	sourcePath, err := SubdirGlob(tmpDir, subDir)
	if err != nil {
		return fmt.Errorf("oci: resolving subdir %q: %w", subDir, err)
	}

	if _, err := g.FS.Stat(sourcePath); err != nil {
		return fmt.Errorf("oci: subdir %q not found in artifact", subDir)
	}

	if err := g.FS.RemoveAll(dst); err != nil {
		return fmt.Errorf("oci: clearing destination %s: %w", dst, err)
	}

	if err := g.FS.MkdirAll(dst, dirPerms); err != nil {
		return fmt.Errorf("oci: creating destination %s: %w", dst, err)
	}

	const manifestFname = ".tgmanifest"

	return copyDir(sourcePath, dst, manifestFname)
}

// newRepository builds an oras remote.Repository for ref (e.g. "ghcr.io/org/repo:v1.0.0").
func (g *OCIGetter) newRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, err
	}

	repo.PlainHTTP = g.PlainHTTP || isLocalhostRegistry(repo.Reference.Registry)
	repo.Client = &auth.Client{
		Client:     g.HTTPClient,
		Credential: ociCredentialFunc(repo.Reference.Registry, g.Logger),
	}

	return repo, nil
}

// isLocalhostRegistry returns true for loopback-address registries.
// oras-go v2 defaults to HTTPS for all registries; loopback addresses are
// treated as plain HTTP so that local dev registries (kind, k3d, airgap
// mirrors) work without TLS configuration.
func isLocalhostRegistry(registry string) bool {
	host, _, _ := net.SplitHostPort(registry)
	if host == "" {
		host = registry
	}

	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// ociCredentialFunc returns an oras credential function for the given registry host.
// Resolution order: TG_OCI_TOKEN → TG_OCI_USERNAME/PASSWORD → Docker config → anonymous.
// l may be nil; debug messages are suppressed when it is.
func ociCredentialFunc(registry string, l log.Logger) auth.CredentialFunc {
	if tok := os.Getenv("TG_OCI_TOKEN"); tok != "" {
		return auth.StaticCredential(registry, auth.Credential{AccessToken: tok})
	}

	user := os.Getenv("TG_OCI_USERNAME")
	pass := os.Getenv("TG_OCI_PASSWORD")

	if user != "" && pass != "" {
		return auth.StaticCredential(registry, auth.Credential{Username: user, Password: pass})
	}

	// Fall through to the Docker credential store, which delegates to any configured
	// credential helper (docker-credential-ecr-login, docker-credential-osxkeychain, etc.).
	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{AllowPlaintextPut: false})
	if err == nil {
		return credentials.Credential(store)
	}

	if l != nil {
		l.Debugf("OCI getter: Docker credential store unavailable (%v); trying anonymous", err)
	}

	return auth.StaticCredential(registry, auth.Credential{})
}

// ociRefFromURL reconstructs the OCI reference string from the parsed URL.
//
//	oci://ghcr.io/org/repo:v1.0.0  →  ghcr.io/org/repo:v1.0.0
func ociRefFromURL(u *url.URL) string {
	return u.Host + u.Path
}

const (
	// gzipPeekBytes is the number of bytes to peek to detect gzip magic.
	gzipPeekBytes = 2
	// gzipMagic0 and gzipMagic1 are the two-byte gzip magic number (RFC 1952).
	gzipMagic0 = 0x1f
	gzipMagic1 = 0x8b

	// dirPerms is the permission mask used when creating directories during extraction.
	dirPerms = 0755
	// filePerms is the default permission mask for extracted regular files.
	filePerms = 0644
)

// extractTarLayer decompresses and untars rc into dst.
// Gzip detection first checks the media type; if that is inconclusive it
// peeks the first two bytes for the gzip magic number (0x1f 0x8b). This
// handles media types like "application/vnd.terraform.module.v1+tar" whose
// name omits "+gzip" but whose bytes are gzip-compressed in practice.
func extractTarLayer(mediaType string, rc io.Reader, dst string) error {
	br := bufio.NewReaderSize(rc, gzipPeekBytes)

	var reader io.Reader = br

	useGzip := isGzipMediaType(mediaType)
	if !useGzip {
		magic, err := br.Peek(gzipPeekBytes)
		if err == nil && len(magic) == gzipPeekBytes && magic[0] == gzipMagic0 && magic[1] == gzipMagic1 {
			useGzip = true
		}
	}

	if useGzip {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("oci: decompressing gzip layer: %w", err)
		}

		defer gz.Close() //nolint:errcheck

		reader = gz
	}

	tr := tar.NewReader(reader)

	if err := os.MkdirAll(dst, dirPerms); err != nil {
		return fmt.Errorf("oci: creating destination %s: %w", dst, err)
	}

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("oci: reading tar: %w", err)
		}

		if err := extractTarEntry(tr, hdr, dst); err != nil {
			return err
		}
	}

	return nil
}

func isGzipMediaType(mediaType string) bool {
	return strings.HasSuffix(mediaType, "+gzip") ||
		strings.HasSuffix(mediaType, ".gzip") ||
		mediaType == "application/gzip"
}

// extractTarEntry writes one tar header+content into dst.
// Directory traversal is prevented by cleaning the entry path.
// Symlinks and special files are skipped.
func extractTarEntry(r io.Reader, hdr *tar.Header, dst string) error {
	// filepath.Join cleans the path; the leading "/" ensures Clean never escapes dst.
	target := filepath.Join(dst, filepath.Clean("/"+hdr.Name)) //nolint:gosec

	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, dirPerms)

	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), dirPerms); err != nil {
			return fmt.Errorf("oci: creating parent for %s: %w", target, err)
		}

		perm := hdr.FileInfo().Mode()
		if perm == 0 {
			perm = filePerms
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			return fmt.Errorf("oci: creating %s: %w", target, err)
		}

		defer f.Close() //nolint:errcheck

		if _, err := io.Copy(f, r); err != nil { //nolint:gosec
			return fmt.Errorf("oci: writing %s: %w", target, err)
		}

	case tar.TypeLink:
		// Hard link: the link target must have been extracted earlier in the archive.
		linkTarget := filepath.Join(dst, filepath.Clean("/"+hdr.Linkname)) //nolint:gosec

		if err := os.MkdirAll(filepath.Dir(target), dirPerms); err != nil {
			return fmt.Errorf("oci: creating parent for hard link %s: %w", target, err)
		}

		if err := os.Link(linkTarget, target); err != nil {
			return fmt.Errorf("oci: creating hard link %s → %s: %w", target, linkTarget, err)
		}
	}

	return nil
}

// NewCASFetchFunc returns a [cas.OCIFetchFunc] backed by an [OCIGetter].
//
// Wire this into [cas.New] via [cas.WithOCIFetch] to enable oci:// sources in
// stack unit/stack CAS processing (ProcessStackComponent). The returned function:
//
//  1. Resolves the manifest digest (deterministic cache key for buildSyntheticTree).
//  2. Extracts the artifact into a fresh temporary directory.
//  3. Returns the directory path, the raw digest string, and a cleanup func.
func NewCASFetchFunc(l log.Logger) cas.OCIFetchFunc {
	return func(ctx context.Context, _ log.Logger, fs vfs.FS, src string) (string, string, func(), error) {
		// Strip any //subdir suffix — fetch the whole artifact.
		baseSrc, _ := SourceDirSubdir(src)

		srcURL, err := url.Parse(baseSrc)
		if err != nil {
			return "", "", nil, fmt.Errorf("oci: parsing source URL %q: %w", baseSrc, err)
		}

		ref := ociRefFromURL(srcURL)
		g := NewOCIGetter(l, fs)

		repo, err := g.newRepository(ref)
		if err != nil {
			return "", "", nil, fmt.Errorf("oci: building repository client for %s: %w", ref, err)
		}

		desc, err := repo.Resolve(ctx, repo.Reference.Reference)
		if err != nil {
			return "", "", nil, fmt.Errorf("oci: resolving %s: %w", ref, err)
		}

		digest := desc.Digest.String() // e.g. "sha256:abc123..."

		tmpDir, err := vfs.MkdirTemp(fs, "", "terragrunt-oci-cas-fetch-")
		if err != nil {
			return "", "", nil, fmt.Errorf("oci: creating temp dir: %w", err)
		}

		cleanup := func() {
			if rmErr := fs.RemoveAll(tmpDir); rmErr != nil {
				l.Warnf("OCI CAS fetch cleanup: %v", rmErr)
			}
		}

		if err := g.Get(ctx, &getter.Request{
			Src:     src,
			Dst:     tmpDir,
			GetMode: getter.ModeDir,
		}); err != nil {
			cleanup()

			return "", "", nil, fmt.Errorf("oci: extracting %q: %w", src, err)
		}

		return tmpDir, digest, cleanup, nil
	}
}

// copyDir walks src and copies all files into dst, skipping skipFile.
func copyDir(src, dst, skipFile string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, dirPerms)
		}

		if filepath.Base(target) == skipFile {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sf, err := os.Open(path) //nolint:gosec
		if err != nil {
			return err
		}

		defer sf.Close() //nolint:errcheck

		df, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}

		defer df.Close() //nolint:errcheck

		_, err = io.Copy(df, sf)

		return err
	})
}
