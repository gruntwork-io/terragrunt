package getter

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

const versionQueryKey = "version"

// RegistryGetter is the go-getter v2 implementation of the tfr:// protocol.
//
// Source URLs take the form:
//
//	tfr://REGISTRY_DOMAIN/MODULE_PATH[?version=VERSION]
//
// where MODULE_PATH is the registry-style namespace/name/system path
// (e.g. terraform-aws-modules/vpc/aws). The `version` query parameter is
// optional; when omitted, the latest stable version is resolved from the
// registry's list-versions endpoint. The getter speaks the Terraform
// Registry Module Registry Protocol
// (https://www.terraform.io/docs/internals/module-registry-protocol.html)
// to resolve the X-Terraform-Get redirect, then re-enters the parent
// go-getter Client (looked up via [getter.ClientFromContext]) to fetch the
// underlying archive. That re-entry is how the v2 RegistryGetter inherits
// the parent's protocol set, headers, and decompressors without keeping a
// stale *Client field around.
//
// Authentication reads TG_TF_REGISTRY_TOKEN from Venv.Env for a bearer token. See
// [tfimpl.DefaultRegistryDomain] and
// [github.com/gruntwork-io/terragrunt/internal/tf/cliconfig] for the rest.
type RegistryGetter struct {
	Logger             log.Logger
	Venv               *venv.Venv
	authCache          *registryAuthCache
	TofuImplementation tfimpl.Type
}

// auth assembles the environment the registry protocol authenticates with.
func (r *RegistryGetter) auth() RegistryAuth {
	return RegistryAuth{Venv: r.Venv, cache: r.authCache}
}

// NewRegistryGetter returns a [RegistryGetter] that issues registry-protocol
// requests and expands archives through v, logs diagnostics to l, and
// defaults to [tfimpl.OpenTofu]. A logger is required because this package
// does not consistently guard against a nil logger, so requiring one at
// construction time prevents nil-pointer panics at call time. Use the With*
// methods to customize other behavior.
func NewRegistryGetter(l log.Logger, v *venv.Venv) *RegistryGetter {
	v.RequireFS()
	v.RequireHTTP()

	return &RegistryGetter{
		Logger:             l,
		Venv:               v,
		TofuImplementation: tfimpl.OpenTofu,
		authCache:          &registryAuthCache{},
	}
}

// WithTofuImplementation selects which default registry domain is used when
// the source URL does not specify a host. See [RegistryGetter.TofuImplementation].
func (r *RegistryGetter) WithTofuImplementation(impl tfimpl.Type) *RegistryGetter {
	r.TofuImplementation = impl
	return r
}

// WithEnv sets the environment the registry auth token is read from.
func (r *RegistryGetter) WithEnv(env map[string]string) *RegistryGetter {
	r.Venv = r.Venv.WithEnv(env)
	r.authCache = &registryAuthCache{}

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
// `:namespace/:name/:system`. The `version` query parameter is optional; when
// absent, the latest stable version is resolved from the registry.
func (r *RegistryGetter) Get(ctx context.Context, req *getter.Request) error {
	srcURL := req.URL()

	registryDomain := srcURL.Host
	if registryDomain == "" {
		registryDomain = tfimpl.DefaultRegistryDomain(r.Venv.Env, r.TofuImplementation)
	}

	queryValues := srcURL.Query()
	modulePath, moduleSubDir := SourceDirSubdir(srcURL.Path)

	moduleRegistryBasePath, err := GetModuleRegistryURLBasePath(
		ctx,
		r.Logger,
		r.Venv.HTTP,
		r.auth(),
		registryDomain,
	)
	if err != nil {
		return err
	}

	version, err := r.resolveVersion(
		ctx,
		queryValues,
		registryDomain,
		moduleRegistryBasePath,
		modulePath,
	)
	if err != nil {
		return err
	}

	moduleURL, err := BuildRequestURL(registryDomain, moduleRegistryBasePath, modulePath, version)
	if err != nil {
		return err
	}

	terraformGet, err := GetTerraformGetHeader(ctx, r.Logger, r.Venv.HTTP, r.auth(), moduleURL)
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
		parent = NewClient(r.Venv)
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
func (r *RegistryGetter) getSubdir(
	ctx context.Context,
	l log.Logger,
	dstPath, sourceURL, subDir string,
) error {
	// Hand the consumer a non-existent path inside an existing parent so
	// go-getter can create the destination itself, and clean up the parent
	// on return.
	parent, err := vfs.MkdirTemp(r.Venv.FS, "", "getter")
	if err != nil {
		return err
	}

	defer func() {
		if err := r.Venv.FS.RemoveAll(parent); err != nil {
			l.Warnf("Error removing temporary directory %s: %v", parent, err)
		}
	}()

	tempdirPath := filepath.Join(parent, "temp")

	if err := r.delegateGet(ctx, tempdirPath, sourceURL); err != nil {
		return fmt.Errorf("downloading registry module archive from %s: %w", sourceURL, err)
	}

	return copySubdirContents(l, r.Venv.FS, tempdirPath, subDir, dstPath, sourceURL)
}

// copySubdirContents resolves subDir under srcRoot and copies its contents
// into dstPath, replacing whatever was there. source only labels errors.
func copySubdirContents(l log.Logger, fsys vfs.FS, srcRoot, subDir, dstPath, source string) error {
	sourcePath, err := SubdirGlob(srcRoot, subDir)
	if err != nil {
		return fmt.Errorf("resolving module subdir %q: %w", subDir, err)
	}

	if _, err := fsys.Stat(sourcePath); err != nil {
		return ModuleDownloadErr{
			sourceURL: source,
			details:   fmt.Sprintf("could not stat download path %s: %s", sourcePath, err),
		}
	}

	if err := fsys.RemoveAll(dstPath); err != nil {
		return fmt.Errorf("clearing destination path %s: %w", dstPath, err)
	}

	const ownerWriteGlobalReadExecutePerms = 0755
	if err := fsys.MkdirAll(dstPath, ownerWriteGlobalReadExecutePerms); err != nil {
		return fmt.Errorf("creating destination path %s: %w", dstPath, err)
	}

	manifestFname := ".tgmanifest"
	manifestPath := filepath.Join(dstPath, manifestFname)

	defer func(name string) {
		if err := fsys.Remove(name); err != nil {
			l.Warnf("Error removing manifest file %s: %v", name, err)
		}
	}(manifestPath)

	return util.CopyFolderContentsWithFilter(
		l,
		fsys,
		sourcePath,
		dstPath,
		manifestFname,
		func(string) bool { return true },
	)
}

// resolveVersion determines the module version to download. If a version is
// specified in the URL query it is validated and returned as-is. Otherwise the
// latest stable version is resolved from the registry's list-versions endpoint.
func (r *RegistryGetter) resolveVersion(
	ctx context.Context,
	queryValues url.Values,
	registryDomain, moduleRegistryBasePath, modulePath string,
) (string, error) {
	versionList, hasVersion := queryValues[versionQueryKey]

	if hasVersion && len(versionList) != 1 {
		return "", MalformedRegistryURLErr{reason: "more than one version query"}
	}

	if hasVersion {
		if versionList[0] == "" {
			return "", MalformedRegistryURLErr{reason: "version query is empty"}
		}

		return versionList[0], nil
	}

	latestVersion, err := GetLatestModuleVersion(
		ctx,
		r.Logger,
		r.Venv.HTTP,
		r.auth(),
		registryDomain,
		moduleRegistryBasePath,
		modulePath,
	)
	if err != nil {
		return "", err
	}

	r.Logger.Infof(
		"No version specified for module %s, using latest version %s",
		modulePath,
		latestVersion,
	)

	return latestVersion, nil
}
