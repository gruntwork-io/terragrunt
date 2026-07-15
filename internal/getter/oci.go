package getter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

const (
	ociTagQueryKey    = "tag"
	ociDigestQueryKey = "digest"
	// ociDefaultTag is the tag used when the source pins neither a tag nor a
	// digest, matching OpenTofu.
	ociDefaultTag = "latest"
	// ociLayerSizeWarnThreshold is the declared layer size above which the
	// getter warns before downloading, pending a hard limit.
	ociLayerSizeWarnThreshold = 1 << 30
)

// ErrOCIGetFileUnsupported reports that oci sources always resolve to module
// directories, so single-file downloads are not supported.
var ErrOCIGetFileUnsupported = errors.New("GetFile is not supported for the OCI Getter")

// ErrOCIGetterNotConfigured reports an OCIGetter used without its NewStore
// seam, logger, or filesystem.
var ErrOCIGetterNotConfigured = errors.New(
	"oci getter is not fully configured (missing repository store, logger, or filesystem). " +
		"This is a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt",
)

// ErrOCIMissingRegistryDomain reports an oci source without a registry host.
var ErrOCIMissingRegistryDomain = errors.New("oci source is missing a registry domain")

// ErrOCIMissingRepositoryName reports an oci source without a repository path.
var ErrOCIMissingRepositoryName = errors.New("oci source is missing a repository name")

// ErrOCITagDigestExclusive reports an oci source that pins both a tag and a
// digest; the wording mirrors OpenTofu so one source string fails the same
// way in both tools.
var ErrOCITagDigestExclusive = errors.New(`cannot set both "tag" and "digest" arguments`)

// OCIUnsupportedQueryParamError reports a query parameter other than tag or
// digest on an oci source.
type OCIUnsupportedQueryParamError struct {
	Param string
}

func (err OCIUnsupportedQueryParamError) Error() string {
	return fmt.Sprintf("unsupported argument %q", err.Param)
}

// OCIArtifactTypeError reports a manifest whose artifact type is not the
// OpenTofu module-package type.
type OCIArtifactTypeError struct {
	ArtifactType string
}

func (err OCIArtifactTypeError) Error() string {
	return fmt.Sprintf("unexpected artifact type %q, expected %q", err.ArtifactType, ArtifactTypeModulePkg)
}

// OCILayerCountError reports a manifest that does not contain exactly one
// module-zip layer.
type OCILayerCountError struct {
	Count int
}

func (err OCILayerCountError) Error() string {
	return fmt.Sprintf("expected exactly one %q layer, found %d", MediaTypeModuleZip, err.Count)
}

// OCIDigestVerificationError reports blob bytes that do not match the digest
// the manifest layer descriptor promised.
type OCIDigestVerificationError struct {
	Err    error
	Digest string
}

func (err OCIDigestVerificationError) Error() string {
	return fmt.Sprintf("verifying blob digest %s: %s", err.Digest, err.Err)
}

func (err OCIDigestVerificationError) Unwrap() error {
	return err.Err
}

// OCIRepositoryStore is the narrow seam between the OCI getter and the
// registry client that serves it. Resolve turns a tag or digest reference
// into a manifest descriptor; Fetch streams the blob a descriptor points at.
// The method set intentionally matches oras-go's content.Fetcher signature so
// manifest and blob helpers work through the seam without adapters, and unit
// tests can drive the getter with a fake store and no network.
type OCIRepositoryStore interface {
	Resolve(ctx context.Context, ref string) (ociv1.Descriptor, error)
	Fetch(ctx context.Context, desc ociv1.Descriptor) (io.ReadCloser, error)
}

// OCINewStoreFunc builds the [OCIRepositoryStore] serving one repository on
// one registry.
type OCINewStoreFunc func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)

// OCIGetter is the go-getter v2 implementation of the oci:// protocol.
//
// Source URLs take the form:
//
//	oci://REGISTRY_DOMAIN/REPOSITORY[//SUBDIR][?tag=TAG|?digest=DIGEST]
//
// where REGISTRY_DOMAIN is an OCI Distribution registry host (host:port
// allowed) and REPOSITORY is the full, possibly multi-segment repository
// name. When neither tag nor digest is set, the tag defaults to latest. The
// manifest contract the getter enforces is centralized in
// [ArtifactTypeModulePkg] and [MediaTypeModuleZip].
//
// NewStore is the dependency-injection seam: credential resolution and
// registry transport live entirely behind it, so credential tiers are
// additive changes and tests inject a fake [OCIRepositoryStore]. The seam
// rides as a struct field rather than a function parameter only because the
// go-getter Getter interface fixes the method set, leaving no parameter to
// thread it through.
type OCIGetter struct {
	NewStore OCINewStoreFunc
	Logger   log.Logger
	FS       vfs.FS
}

var _ getter.Getter = (*OCIGetter)(nil)

// Mode reports directory mode for all oci sources, since oci always
// downloads a module directory.
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

// Get downloads the module package at req.Src into req.Dst. It resolves the
// tag or digest to a manifest, requires the [ArtifactTypeModulePkg] artifact
// type, streams the single [MediaTypeModuleZip] layer with digest
// verification, and extracts it, honoring a //SUBDIR selector.
func (g *OCIGetter) Get(ctx context.Context, req *getter.Request) error {
	if g.NewStore == nil || g.Logger == nil || g.FS == nil {
		return ErrOCIGetterNotConfigured
	}

	srcURL := req.URL()

	registryDomain := srcURL.Host
	if registryDomain == "" {
		return ErrOCIMissingRegistryDomain
	}

	repositoryName, subDir := SourceDirSubdir(strings.TrimPrefix(srcURL.Path, "/"))
	if repositoryName == "" {
		return ErrOCIMissingRepositoryName
	}

	ref, err := ociRefFromQuery(srcURL.Query())
	if err != nil {
		return err
	}

	store, err := g.NewStore(ctx, registryDomain, repositoryName)
	if err != nil {
		return fmt.Errorf("creating OCI repository store for %s/%s: %w", registryDomain, repositoryName, err)
	}

	layer, err := resolveModuleZipLayer(ctx, store, ref)
	if err != nil {
		return err
	}

	if layer.Size > ociLayerSizeWarnThreshold {
		g.Logger.Warnf(
			"OCI layer %s declares %d bytes, above the %d byte threshold; downloading it may be slow",
			layer.Digest, layer.Size, ociLayerSizeWarnThreshold,
		)
	}

	// Hand extraction a temp parent so a partial download never lands in
	// req.Dst, and clean up the parent on return.
	parent, err := vfs.MkdirTemp(g.FS, "", "getter")
	if err != nil {
		return err
	}

	defer func() {
		if err := g.FS.RemoveAll(parent); err != nil {
			g.Logger.Warnf("Error removing temporary directory %s: %v", parent, err)
		}
	}()

	zipPath, err := g.fetchModuleZip(ctx, store, &layer, parent)
	if err != nil {
		return err
	}

	// Extract into a staging directory and replace req.Dst only after full
	// success, so a malformed archive never corrupts an existing destination
	// and files removed between module versions do not survive.
	// go-getter's client strips the //subdir selector before Get, so subDir
	// is empty in production; the client copies the requested subdir out of
	// req.Dst afterward. copySubdirContents handles a direct call too.
	unzipPath := filepath.Join(parent, "unzip")
	if err := (&getter.ZipDecompressor{}).Decompress(unzipPath, zipPath, true, req.Umask); err != nil {
		return fmt.Errorf("extracting OCI module archive: %w", err)
	}

	return copySubdirContents(g.Logger, g.FS, unzipPath, subDir, req.Dst, req.Src)
}

// GetFile always fails, per [ErrOCIGetFileUnsupported].
func (g *OCIGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return ErrOCIGetFileUnsupported
}

// ociRefFromQuery validates the source query and returns the reference to
// resolve: the digest when pinned, the tag otherwise, defaulting to
// [ociDefaultTag] when neither is set.
func ociRefFromQuery(queryValues url.Values) (string, error) {
	for param := range queryValues {
		if param != ociTagQueryKey && param != ociDigestQueryKey {
			return "", OCIUnsupportedQueryParamError{Param: param}
		}
	}

	tag := queryValues.Get(ociTagQueryKey)
	digest := queryValues.Get(ociDigestQueryKey)

	if tag != "" && digest != "" {
		return "", ErrOCITagDigestExclusive
	}

	if digest != "" {
		return digest, nil
	}

	if tag != "" {
		return tag, nil
	}

	return ociDefaultTag, nil
}

// resolveModuleZipLayer resolves ref to a manifest, enforces the module
// package contract, and returns the single module-zip layer descriptor.
func resolveModuleZipLayer(ctx context.Context, store OCIRepositoryStore, ref string) (ociv1.Descriptor, error) {
	manifestDesc, err := store.Resolve(ctx, ref)
	if err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("resolving OCI reference %q: %w", ref, err)
	}

	manifestBytes, err := content.FetchAll(ctx, store, manifestDesc)
	if err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("fetching OCI manifest %s: %w", manifestDesc.Digest, err)
	}

	var manifest ociv1.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("parsing OCI manifest %s: %w", manifestDesc.Digest, err)
	}

	if manifest.ArtifactType != ArtifactTypeModulePkg {
		return ociv1.Descriptor{}, OCIArtifactTypeError{ArtifactType: manifest.ArtifactType}
	}

	var zipLayers []ociv1.Descriptor

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeModuleZip {
			zipLayers = append(zipLayers, layer)
		}
	}

	if len(zipLayers) != 1 {
		return ociv1.Descriptor{}, OCILayerCountError{Count: len(zipLayers)}
	}

	return zipLayers[0], nil
}

// fetchModuleZip streams the layer blob into a zip file under parent,
// verifying the bytes against the layer descriptor digest, and returns the
// file path.
func (g *OCIGetter) fetchModuleZip(
	ctx context.Context,
	store OCIRepositoryStore,
	layer *ociv1.Descriptor,
	parent string,
) (string, error) {
	blob, err := store.Fetch(ctx, *layer)
	if err != nil {
		return "", fmt.Errorf("fetching OCI layer %s: %w", layer.Digest, err)
	}

	defer func() {
		if err := blob.Close(); err != nil {
			g.Logger.Warnf("Error closing OCI layer blob %s: %v", layer.Digest, err)
		}
	}()

	zipFile, err := vfs.CreateTemp(g.FS, parent, "module-*.zip")
	if err != nil {
		return "", err
	}

	verifyReader := content.NewVerifyReader(blob, *layer)

	if _, err := io.Copy(zipFile, verifyReader); err != nil {
		if closeErr := zipFile.Close(); closeErr != nil {
			g.Logger.Warnf("Error closing temporary file %s: %v", zipFile.Name(), closeErr)
		}

		return "", fmt.Errorf("streaming OCI layer %s: %w", layer.Digest, err)
	}

	if err := zipFile.Close(); err != nil {
		return "", err
	}

	if err := verifyReader.Verify(); err != nil {
		return "", OCIDigestVerificationError{Digest: layer.Digest.String(), Err: err}
	}

	return zipFile.Name(), nil
}
