package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ModuleManifestName is the manifest for files copied from terragrunt module folder (i.e., the folder that contains the current terragrunt.hcl).
const (
	ModuleManifestName = ".terragrunt-module-manifest"

	// ModuleInitRequiredFile is a file to indicate that init should be executed.
	ModuleInitRequiredFile = ".terragrunt-init-required"

	tfLintConfig = ".tflint.hcl"

	fileURIScheme = "file://"
)

// DownloadTerraformSource downloads the given source URL, which should use Terraform's module source syntax,
// into a temporary folder, then:
// 1. Check if module directory exists in temporary folder
// 2. Copy the contents of opts.WorkingDir into the temporary folder.
// 3. Set opts.WorkingDir to the temporary folder.
//
// See the NewTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func DownloadTerraformSource(
	ctx context.Context,
	l log.Logger,
	source string,
	opts *Options,
	cfg *runcfg.RunConfig,
	r *report.Report,
) (*Options, error) {
	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

	source = tf.RewriteLegacyGCSPublicSource(ctx, l, source, opts.StrictControls)

	terraformSource, err := tf.NewSource(l, source, opts.DownloadDir, opts.WorkingDir, walkWithSymlinks)
	if err != nil {
		return nil, err
	}

	// Serialize concurrent downloads to the same cache directory. Without this,
	// manifest.Clean() in one goroutine can delete files while another goroutine
	// is checking for them (e.g. during CheckFolderContainsTerraformCode).
	rawLock, _ := sourceChangeLocks.LoadOrStore(terraformSource.DownloadDir, &sync.Mutex{})
	dirLock := rawLock.(*sync.Mutex)
	dirLock.Lock()
	defer dirLock.Unlock()

	downloaded, err := DownloadTerraformSourceIfNecessary(ctx, l, terraformSource, opts, cfg, r)
	if err != nil {
		return nil, err
	}

	// When no download was needed (AlreadyHaveLatestCode=true) and the source
	// directory IS the working directory (source="."), skip the module copy: the
	// version hash incorporates all file mod times, so no files have changed and
	// the cache already has the correct content from a previous run. Skipping
	// avoids manifest.Clean() deleting files that a concurrent goroutine expects
	// to exist.
	//
	// When the source is a different directory (local or remote), the module copy
	// overlays working-dir files on top of the downloaded source. These files may
	// change independently of the source version hash, so the copy must always run.
	sourceIsWorkingDir := tf.IsLocalSource(terraformSource.CanonicalSourceURL) &&
		filepath.Clean(terraformSource.CanonicalSourceURL.Path) == filepath.Clean(opts.WorkingDir)
	needsModuleCopy := downloaded || !sourceIsWorkingDir

	if needsModuleCopy {
		l.Debugf(
			"Copying files from %s into %s",
			util.RelPathForLog(opts.WorkingDir, opts.WorkingDir, opts.Writers.LogShowAbsPaths),
			util.RelPathForLog(opts.RootWorkingDir, terraformSource.WorkingDir, opts.Writers.LogShowAbsPaths),
		)

		// Always include the .tflint.hcl file, if it exists
		includeInCopy := slices.Concat(cfg.Terraform.IncludeInCopy, []string{tfLintConfig})

		copyOpts := []util.CopyOption{
			util.WithIncludeInCopy(includeInCopy...),
			util.WithExcludeFromCopy(cfg.Terraform.ExcludeFromCopy...),
		}
		if controls.IsFastCopyEnabled(opts.StrictControls) {
			copyOpts = append(copyOpts, util.WithFastCopy())
		}

		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "copy_folder_contents", map[string]any{
			"src":  opts.WorkingDir,
			"dest": terraformSource.WorkingDir,
		}, func(_ context.Context) error {
			return util.CopyFolderContents(l, opts.WorkingDir, terraformSource.WorkingDir, ModuleManifestName, copyOpts...)
		})
		if err != nil {
			return nil, err
		}
	}

	l, updatedOpts, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	l.Debugf(
		"Setting working directory to %s",
		util.RelPathForLog(
			opts.RootWorkingDir,
			terraformSource.WorkingDir,
			opts.Writers.LogShowAbsPaths,
		),
	)
	updatedOpts.WorkingDir = terraformSource.WorkingDir

	return updatedOpts, nil
}

// DownloadTerraformSourceIfNecessary downloads the specified TerraformSource if the latest code hasn't already been
// downloaded. It returns true if a download was performed, or false if the existing cache was up to date.
func DownloadTerraformSourceIfNecessary(
	ctx context.Context,
	l log.Logger,
	terraformSource *tf.Source,
	opts *Options,
	cfg *runcfg.RunConfig,
	r *report.Report,
) (bool, error) {
	if opts.SourceUpdate {
		l.Debugf("The --source-update flag is set, so deleting the temporary folder %s before downloading source.", terraformSource.DownloadDir)

		if err := os.RemoveAll(terraformSource.DownloadDir); err != nil {
			return false, errors.New(err)
		}
	} else {
		alreadyLatest, err := AlreadyHaveLatestCode(l, terraformSource, opts)
		if err != nil {
			return false, err
		}

		if alreadyLatest {
			if err := ValidateWorkingDir(terraformSource); err != nil {
				return false, err
			}

			l.Debugf(
				"%s files in %s are up to date. Will not download again.",
				opts.TofuImplementation,
				util.RelPathForLog(
					opts.RootWorkingDir,
					terraformSource.WorkingDir,
					opts.Writers.LogShowAbsPaths,
				),
			)

			return false, nil
		}
	}

	var previousVersion = ""
	// read previous source version
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if util.FileExists(terraformSource.VersionFile) {
		var err error

		previousVersion, err = readVersionFile(terraformSource)
		if err != nil {
			return false, err
		}
	}

	// When downloading source, we need to process any hooks waiting on `init-from-module`. Therefore, we clone the
	// options struct, set the command to the value the hooks are expecting, and run the download action surrounded by
	// before and after hooks (if any).
	l, optsForDownload, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return false, err
	}

	optsForDownload.TerraformCommand = tf.CommandNameInitFromModule

	downloadErr := RunActionWithHooks(
		ctx,
		l,
		"download source",
		optsForDownload,
		cfg,
		r,
		func(childCtx context.Context) error {
			if opts.Experiments.Evaluate(experiment.SlowTaskReporting) {
				sourceURL := strings.TrimPrefix(terraformSource.CanonicalSourceURL.String(), fileURIScheme)

				return util.NotifyIfSlow(childCtx, l, util.SpinnerWriter(), time.Second, util.SlowNotifyMsg{
					Spinner: "Downloading source from " + sourceURL + "...",
					Done:    "Downloaded source from " + sourceURL,
				}, func() error {
					return downloadSource(childCtx, l, terraformSource, opts, cfg, r)
				})
			}

			return downloadSource(childCtx, l, terraformSource, opts, cfg, r)
		},
	)
	if downloadErr != nil {
		return false, DownloadingTerraformSourceErr{ErrMsg: downloadErr, URL: terraformSource.CanonicalSourceURL.String()}
	}

	if err := terraformSource.WriteVersionFile(l); err != nil {
		return false, err
	}

	if err := ValidateWorkingDir(terraformSource); err != nil {
		return false, err
	}

	currentVersion, err := terraformSource.EncodeSourceVersion(l)
	// if source versions are different or calculating version failed, create file to run init
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if (previousVersion != "" && previousVersion != currentVersion) || err != nil {
		l.Debugf("Requesting re-init, source version has changed from %s to %s recently.", previousVersion, currentVersion)

		initFile := filepath.Join(terraformSource.WorkingDir, ModuleInitRequiredFile)

		f, createErr := os.Create(initFile)
		if createErr != nil {
			return false, createErr
		}

		defer f.Close()
	}

	return true, nil
}

// AlreadyHaveLatestCode returns true if the specified TerraformSource, of the exact same version, has already been downloaded into the
// DownloadFolder. This helps avoid downloading the same code multiple times. Note that if the TerraformSource points
// to a local file path, a hash will be generated from the contents of the source dir. See the ProcessTerraformSource method for more info.
func AlreadyHaveLatestCode(l log.Logger, terraformSource *tf.Source, opts *Options) (bool, error) {
	if !util.FileExists(terraformSource.DownloadDir) ||
		!util.FileExists(terraformSource.WorkingDir) ||
		!util.FileExists(terraformSource.VersionFile) {
		return false, nil
	}

	hasFiles, err := util.DirContainsTFFiles(terraformSource.WorkingDir)
	if err != nil {
		return false, errors.New(err)
	}

	if !hasFiles {
		l.Debugf("Working dir %s exists but contains no Terraform or OpenTofu files, so assuming code needs to be downloaded again.", terraformSource.WorkingDir)
		return false, nil
	}

	currentVersion, err := terraformSource.EncodeSourceVersion(l)
	// If we fail to calculate the source version (e.g. because walking the
	// directory tree failed) use a random version instead, bypassing the cache.
	if err != nil {
		currentVersion, err = util.GenerateRandomSha256()
		if err != nil {
			return false, err
		}
	}

	previousVersion, err := readVersionFile(terraformSource)
	if err != nil {
		return false, err
	}

	return previousVersion == currentVersion, nil
}

// Return the version number stored in the DownloadDir. This version number can be used to check if the Terraform code
// that has already been downloaded is the same as the version the user is currently requesting. The version number is
// calculated using the encodeSourceVersion method.
func readVersionFile(terraformSource *tf.Source) (string, error) {
	return util.ReadFileAsString(terraformSource.VersionFile)
}

// downloadSource downloads the canonical source URL into src.DownloadDir.
//
// When CAS is enabled and the source is remote, it tries the CAS-only client
// first (cas::sha1:<hash> + git-via-CAS). On CAS failure or for local
// sources, it falls through to the standard Terragrunt-configured client
// from internal/getter, which registers the full default protocol set
// (s3, gcs, git, hg, smb, http(s), file) plus the FileCopy and tfr
// customizations.
func downloadSource(
	ctx context.Context,
	l log.Logger,
	src *tf.Source,
	opts *Options,
	cfg *runcfg.RunConfig,
	r *report.Report,
) error {
	canonicalSourceURL := src.CanonicalSourceURL.String()

	// Strip file:// so file://../../path/to/dir doesn't show up in user-facing logs.
	canonicalSourceURL = strings.TrimPrefix(canonicalSourceURL, fileURIScheme)

	l.Infof(
		"Downloading Terraform configurations from %s into %s",
		util.RelPathForLog(opts.RootWorkingDir, canonicalSourceURL, opts.Writers.LogShowAbsPaths),
		util.RelPathForLog(opts.RootWorkingDir, src.DownloadDir, opts.Writers.LogShowAbsPaths))

	allowCAS := opts.Experiments.Evaluate(experiment.CAS) && !opts.NoCAS

	if cfg.Terraform.UpdateSourceWithCAS && !allowCAS {
		return errors.New(&cas.UpdateSourceWithCASRequiresCASError{
			BlockType: "terraform",
			Path:      opts.TerragruntConfigPath,
		})
	}

	isLocalSource := tf.IsLocalSource(src.CanonicalSourceURL)

	if allowCAS && !isLocalSource {
		done, err := tryCASDownload(ctx, l, src, opts, cfg.Terraform.Mutable)
		if err != nil {
			return err
		}

		if done {
			return nil
		}
	}

	return opts.RunWithErrorHandling(ctx, l, r, func() error {
		client := BuildDownloadClient(l, opts, cfg)

		_, err := client.Get(ctx, &getter.Request{
			Src:     src.CanonicalSourceURL.String(),
			Dst:     src.DownloadDir,
			GetMode: getter.ModeAny,
		})

		return err
	})
}

// tryCASDownload attempts a CAS-backed fetch.
//
// Returns (true, nil) on success. Caller is done.
// Returns (false, nil) when the CAS path could not be taken but the failure
// is recoverable (CAS init failure, CAS-getter download failure). Caller
// should fall through to the standard getter.
// Returns (false, err) for fatal misconfiguration the user must fix
// (e.g. an invalid CASCloneDepth). Caller must propagate the error.
func tryCASDownload(ctx context.Context, l log.Logger, src *tf.Source, opts *Options, mutable bool) (bool, error) {
	canonicalSourceURL := src.CanonicalSourceURL.String()

	l.Debugf("CAS experiment enabled: attempting to use Content Addressable Storage for source: %s", canonicalSourceURL)

	if err := cas.ValidateCASCloneDepth(opts.CASCloneDepth); err != nil {
		return false, err
	}

	c, err := cas.New(cas.WithCloneDepth(opts.CASCloneDepth))
	if err != nil {
		l.Warnf("Failed to initialize CAS: %v. Falling back to standard getter.", err)
		return false, nil
	}

	venv, err := cas.OSVenv()
	if err != nil {
		l.Warnf("Failed to initialize CAS environment: %v. Falling back to standard getter.", err)
		return false, nil
	}

	cloneOpts := cas.CloneOptions{
		Dir:              src.DownloadDir,
		IncludedGitFiles: []string{"HEAD", "config"},
		Mutable:          mutable,
	}

	casProtocol := getter.NewCASProtocolGetter(l, c, venv)
	casProtocol.Mutable = mutable

	// CAS-only client: CASProtocolGetter handles cas::sha1:<hash> sources
	// (from CAS-rewritten stacks); CASGetter handles git:: and other remote
	// sources via CAS. No fallback getters here, so a failure means the
	// caller should retry through the standard client.
	client := &getter.Client{
		Getters: []getter.Getter{
			casProtocol,
			getter.NewCASGetter(l, c, venv, &cloneOpts, getter.WithDefaultGenericDispatch(
				getter.WithTFRConfig(l, opts.TofuImplementation),
			)),
		},
	}

	if _, err := client.Get(ctx, &getter.Request{
		Src: canonicalSourceURL,
		Dst: src.DownloadDir,
		Pwd: opts.WorkingDir,
	}); err != nil {
		l.Warnf("CAS download failed: %v. Falling back to standard getter.", err)

		// Clear any partial CAS output before the fallback runs; mixing
		// leftover CAS files with the standard getter's output leaves the
		// module dir in an inconsistent state.
		if removeErr := os.RemoveAll(src.DownloadDir); removeErr != nil {
			l.Warnf("Failed to clean partial CAS output at %s: %v", src.DownloadDir, removeErr)
		}

		return false, nil
	}

	l.Debugf("Successfully downloaded source using CAS: %s", canonicalSourceURL)

	return true, nil
}

// BuildDownloadClient constructs the go-getter client used for the standard
// (non-CAS) download path. The customizations layered on top of the default
// protocol set are: FileCopyGetter (copies local sources instead of
// symlinking) and RegistryGetter (resolves tfr:// sources).
//
// Exported so tests can assert the protocol set directly.
func BuildDownloadClient(l log.Logger, opts *Options, cfg *runcfg.RunConfig) *getter.Client {
	return getter.NewClient(
		getter.WithLogger(l),
		getter.WithFileCopy(getter.NewFileCopyGetter().
			WithLogger(l).
			WithIncludeInCopy(cfg.Terraform.IncludeInCopy...).
			WithExcludeFromCopy(cfg.Terraform.ExcludeFromCopy...).
			WithFastCopy(controls.IsFastCopyEnabled(opts.StrictControls))),
		getter.WithTFRegistry(getter.NewRegistryGetter(l).
			WithTofuImplementation(opts.TofuImplementation)),
	)
}

// ValidateWorkingDir checks if working terraformSource.WorkingDir exists and is a directory
func ValidateWorkingDir(terraformSource *tf.Source) error {
	workingLocalDir := strings.ReplaceAll(terraformSource.WorkingDir, terraformSource.DownloadDir+filepath.FromSlash("/"), "")
	if util.IsFile(terraformSource.WorkingDir) {
		return WorkingDirNotDir{Dir: workingLocalDir, Source: terraformSource.CanonicalSourceURL.String()}
	}

	if !util.IsDir(terraformSource.WorkingDir) {
		return WorkingDirNotFound{Dir: workingLocalDir, Source: terraformSource.CanonicalSourceURL.String()}
	}

	return nil
}

type WorkingDirNotFound struct {
	Source string
	Dir    string
}

func (err WorkingDirNotFound) Error() string {
	return fmt.Sprintf("Working dir %s from source %s does not exist", err.Dir, err.Source)
}

type WorkingDirNotDir struct {
	Source string
	Dir    string
}

func (err WorkingDirNotDir) Error() string {
	return fmt.Sprintf("Valid working dir %s from source %s", err.Dir, err.Source)
}

type DownloadingTerraformSourceErr struct {
	ErrMsg error
	URL    string
}

func (err DownloadingTerraformSourceErr) Error() string {
	return fmt.Sprintf("downloading source url %s\n%v", err.URL, err.ErrMsg)
}

func (err DownloadingTerraformSourceErr) Unwrap() error {
	return err.ErrMsg
}
