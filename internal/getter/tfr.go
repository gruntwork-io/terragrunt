package getter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-cleanhttp"
	getter "github.com/hashicorp/go-getter/v2"
	safetemp "github.com/hashicorp/go-safetemp"
)

const versionQueryKey = "version"

// RegistryGetter is the go-getter v2 implementation of the tfr:// protocol.
//
// Source URLs take the form:
//
//	tfr://REGISTRY_DOMAIN/MODULE_PATH?version=VERSION
//
// where MODULE_PATH is the registry-style namespace/name/system path
// (e.g. terraform-aws-modules/vpc/aws). The getter speaks the
// Terraform Registry Module Registry Protocol
// (https://www.terraform.io/docs/internals/module-registry-protocol.html)
// to resolve the X-Terraform-Get redirect, then re-enters the parent
// go-getter Client (looked up via [getter.ClientFromContext]) to fetch the
// underlying archive. That re-entry is how the v2 RegistryGetter inherits
// the parent's protocol set, headers, and decompressors without keeping a
// stale *Client field around.
//
// Authentication uses environment variables: TG_TF_REGISTRY_TOKEN supplies a
// bearer token. See [tfimpl.DefaultRegistryDomain] and
// [github.com/gruntwork-io/terragrunt/internal/tf/cliconfig] for the rest.
type RegistryGetter struct {
	HTTPClient         *http.Client
	Logger             log.Logger
	FS                 vfs.FS
	TofuImplementation tfimpl.Type
}

// NewRegistryGetter returns a [RegistryGetter] configured with sensible
// defaults: a [github.com/hashicorp/go-cleanhttp.DefaultClient] for
// registry-protocol requests, the supplied logger for diagnostic output, and
// [tfimpl.OpenTofu] as the default implementation. A logger is required
// because this package does not consistently guard against a nil logger, so
// requiring one at construction time prevents nil-pointer panics at call time.
// Use the With* methods to customize other behavior.
func NewRegistryGetter(l log.Logger) *RegistryGetter {
	return &RegistryGetter{
		HTTPClient:         cleanhttp.DefaultClient(),
		Logger:             l,
		FS:                 vfs.NewOSFS(),
		TofuImplementation: tfimpl.OpenTofu,
	}
}

// WithHTTPClient overrides the HTTP client used for registry-protocol
// requests. Intended for tests that need to route requests through a
// [net/http/httptest.Server], or for callers that need custom transport
// configuration.
func (r *RegistryGetter) WithHTTPClient(c *http.Client) *RegistryGetter {
	r.HTTPClient = c
	return r
}

// WithTofuImplementation selects which default registry domain is used when
// the source URL does not specify a host. See [RegistryGetter.TofuImplementation].
func (r *RegistryGetter) WithTofuImplementation(impl tfimpl.Type) *RegistryGetter {
	r.TofuImplementation = impl
	return r
}

// Mode reports directory mode for all tfr sources, since tfr always
// downloads a module directory.
func (r *RegistryGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect recognizes tfr:// sources and tfr-forced sources.
func (r *RegistryGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == "tfr" {
		return true, nil
	}

	u, err := url.Parse(req.Src)
	if err == nil && u.Scheme == "tfr" {
		return true, nil
	}

	return false, nil
}

// Get fetches the module contents specified at req.Src and downloads them to
// req.Dst. req.Src must be a tfr:// URL with the module path encoded as
// `:namespace/:name/:system` and a `version` query parameter.
func (r *RegistryGetter) Get(ctx context.Context, req *getter.Request) error {
	srcURL := req.URL()

	registryDomain := srcURL.Host
	if registryDomain == "" {
		registryDomain = tfimpl.DefaultRegistryDomain(r.TofuImplementation)
	}

	queryValues := srcURL.Query()
	modulePath, moduleSubDir := SourceDirSubdir(srcURL.Path)

	versionList, hasVersion := queryValues[versionQueryKey]
	if !hasVersion {
		return errors.New(MalformedRegistryURLErr{reason: "missing version query"})
	}

	if len(versionList) != 1 {
		return errors.New(MalformedRegistryURLErr{reason: "more than one version query"})
	}

	version := versionList[0]

	moduleRegistryBasePath, err := GetModuleRegistryURLBasePath(ctx, r.Logger, r.HTTPClient, registryDomain)
	if err != nil {
		return err
	}

	moduleURL, err := BuildRequestURL(registryDomain, moduleRegistryBasePath, modulePath, version)
	if err != nil {
		return err
	}

	terraformGet, err := GetTerraformGetHeader(ctx, r.Logger, r.HTTPClient, moduleURL)
	if err != nil {
		return err
	}

	downloadURL, err := GetDownloadURLFromHeader(moduleURL, terraformGet)
	if err != nil {
		return err
	}

	source, subDir := SourceDirSubdir(downloadURL)
	if subDir == "" && moduleSubDir == "" {
		return r.delegateGet(ctx, req.Dst, source)
	}

	// Subdir present: download the root into a temp dir then copy out the
	// requested subdirectory.
	return r.getSubdir(ctx, r.Logger, req.Dst, source, path.Join(subDir, moduleSubDir))
}

// GetFile is not implemented for the Terraform module registry Getter since
// the terraform module registry doesn't serve a single file.
func (r *RegistryGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return errors.New("GetFile is not implemented for the Terraform Registry Getter")
}

// delegateGet re-enters the parent client so the actual archive download
// inherits its protocol set, decompressors, and headers. When called outside
// a Client.Get tree (e.g. from a top-level helper) it falls back to a fresh
// client built without the tfr getter to prevent infinite recursion.
func (r *RegistryGetter) delegateGet(ctx context.Context, dst, src string) error {
	parent := getter.ClientFromContext(ctx)
	if parent == nil {
		parent = NewClient()
	}

	_, err := parent.Get(ctx, &getter.Request{
		Src:     src,
		Dst:     dst,
		GetMode: getter.ModeDir,
	})

	return err
}

// getSubdir downloads the source into a temp dir, then copies the requested
// subdirectory into dstPath. This is how the registry getter handles
// `MODULE/subdir` selectors.
func (r *RegistryGetter) getSubdir(ctx context.Context, l log.Logger, dstPath, sourceURL, subDir string) error {
	tempdirPath, tempdirCloser, err := safetemp.Dir("", "getter")
	if err != nil {
		return err
	}

	defer func(tempdirCloser io.Closer) {
		if err := tempdirCloser.Close(); err != nil {
			l.Warnf("Error closing temporary directory %s: %v", tempdirPath, err)
		}
	}(tempdirCloser)

	if err := r.delegateGet(ctx, tempdirPath, sourceURL); err != nil {
		return fmt.Errorf("downloading registry module archive from %s: %w", sourceURL, err)
	}

	sourcePath, err := SubdirGlob(tempdirPath, subDir)
	if err != nil {
		return fmt.Errorf("resolving registry module subdir %q: %w", subDir, err)
	}

	if _, err := r.FS.Stat(sourcePath); err != nil {
		return errors.New(ModuleDownloadErr{
			sourceURL: sourceURL,
			details:   fmt.Sprintf("could not stat download path %s: %s", sourcePath, err),
		})
	}

	if err := r.FS.RemoveAll(dstPath); err != nil {
		return fmt.Errorf("clearing destination path %s: %w", dstPath, err)
	}

	const ownerWriteGlobalReadExecutePerms = 0755
	if err := r.FS.MkdirAll(dstPath, ownerWriteGlobalReadExecutePerms); err != nil {
		return fmt.Errorf("creating destination path %s: %w", dstPath, err)
	}

	manifestFname := ".tgmanifest"
	manifestPath := filepath.Join(dstPath, manifestFname)

	defer func(name string) {
		if err := r.FS.Remove(name); err != nil {
			l.Warnf("Error removing temporary directory %s: %v", name, err)
		}
	}(manifestPath)

	return util.CopyFolderContentsWithFilter(l, sourcePath, dstPath, manifestFname, func(string) bool { return true })
}
