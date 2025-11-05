package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter"
	getterv2 "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// ModuleManifestName is the manifest for files copied from terragrunt module folder (i.e., the folder that contains the current terragrunt.hcl).
const (
	ModuleManifestName = ".terragrunt-module-manifest"

	// ModuleInitRequiredFile is a file to indicate that init should be executed.
	ModuleInitRequiredFile = ".terragrunt-init-required"

	tfLintConfig = ".tflint.hcl"

	fileURIScheme = "file://"
)

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Check if module directory exists in temporary folder
// 3. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 4. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the NewTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformSource(
	ctx context.Context,
	l log.Logger,
	source string,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	r *report.Report,
) (*options.TerragruntOptions, error) {
	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

	terraformSource, err := tf.NewSource(l, source, opts.DownloadDir, opts.WorkingDir, walkWithSymlinks)
	if err != nil {
		return nil, err
	}

	if err = DownloadTerraformSourceIfNecessary(ctx, l, terraformSource, opts, cfg, r); err != nil {
		return nil, err
	}

	l.Debugf("Copying files from %s into %s", opts.WorkingDir, terraformSource.WorkingDir)

	var includeInCopy, excludeFromCopy []string

	if cfg.Terraform != nil && cfg.Terraform.IncludeInCopy != nil {
		includeInCopy = *cfg.Terraform.IncludeInCopy
	}

	if cfg.Terraform != nil && cfg.Terraform.ExcludeFromCopy != nil {
		excludeFromCopy = *cfg.Terraform.ExcludeFromCopy
	}

	// Always include the .tflint.hcl file, if it exists
	includeInCopy = append(includeInCopy, tfLintConfig)

	err = util.CopyFolderContents(
		l,
		opts.WorkingDir,
		terraformSource.WorkingDir,
		ModuleManifestName,
		includeInCopy,
		excludeFromCopy,
	)
	if err != nil {
		return nil, err
	}

	l, updatedTerragruntOptions, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	l.Debugf("Setting working directory to %s", terraformSource.WorkingDir)
	updatedTerragruntOptions.WorkingDir = terraformSource.WorkingDir

	return updatedTerragruntOptions, nil
}

// DownloadTerraformSourceIfNecessary downloads the specified TerraformSource if the latest code hasn't already been downloaded.
func DownloadTerraformSourceIfNecessary(
	ctx context.Context,
	l log.Logger,
	terraformSource *tf.Source,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	r *report.Report,
) error {
	if opts.SourceUpdate {
		l.Debugf("The --source-update flag is set, so deleting the temporary folder %s before downloading source.", terraformSource.DownloadDir)

		if err := os.RemoveAll(terraformSource.DownloadDir); err != nil {
			return errors.New(err)
		}
	} else {
		alreadyLatest, err := AlreadyHaveLatestCode(l, terraformSource, opts)
		if err != nil {
			return err
		}

		if alreadyLatest {
			if err := ValidateWorkingDir(terraformSource); err != nil {
				return err
			}

			l.Debugf("%s files in %s are up to date. Will not download again.", opts.TerraformImplementation, terraformSource.WorkingDir)

			return nil
		}
	}

	var previousVersion = ""
	// read previous source version
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if util.FileExists(terraformSource.VersionFile) {
		var err error

		previousVersion, err = readVersionFile(terraformSource)
		if err != nil {
			return err
		}
	}

	// When downloading source, we need to process any hooks waiting on `init-from-module`. Therefore, we clone the
	// options struct, set the command to the value the hooks are expecting, and run the download action surrounded by
	// before and after hooks (if any).
	l, terragruntOptionsForDownload, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return err
	}

	terragruntOptionsForDownload.TerraformCommand = tf.CommandNameInitFromModule

	downloadErr := RunActionWithHooks(ctx, l, "download source", terragruntOptionsForDownload, cfg, r, func(_ context.Context) error {
		return downloadSource(ctx, l, terraformSource, opts, cfg, r)
	})
	if downloadErr != nil {
		return DownloadingTerraformSourceErr{ErrMsg: downloadErr, URL: terraformSource.CanonicalSourceURL.String()}
	}

	if err := terraformSource.WriteVersionFile(l); err != nil {
		return err
	}

	if err := ValidateWorkingDir(terraformSource); err != nil {
		return err
	}

	currentVersion, err := terraformSource.EncodeSourceVersion(l)
	// if source versions are different or calculating version failed, create file to run init
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if (previousVersion != "" && previousVersion != currentVersion) || err != nil {
		l.Debugf("Requesting re-init, source version has changed from %s to %s recently.", previousVersion, currentVersion)

		initFile := util.JoinPath(terraformSource.WorkingDir, ModuleInitRequiredFile)

		f, createErr := os.Create(initFile)
		if createErr != nil {
			return createErr
		}

		defer f.Close()
	}

	return nil
}

// AlreadyHaveLatestCode returns true if the specified TerraformSource, of the exact same version, has already been downloaded into the
// DownloadFolder. This helps avoid downloading the same code multiple times. Note that if the TerraformSource points
// to a local file path, a hash will be generated from the contents of the source dir. See the ProcessTerraformSource method for more info.
func AlreadyHaveLatestCode(l log.Logger, terraformSource *tf.Source, opts *options.TerragruntOptions) (bool, error) {
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

// UpdateGetters returns the customized go-getter interfaces that Terragrunt relies on. Specifically:
//   - Local file path getter is updated to copy the files instead of creating symlinks, which is what go-getter defaults
//     to.
//   - Include the customized getter for fetching sources from the Terraform Registry.
//
// This creates a closure that returns a function so that we have access to the terragrunt configuration, which is
// necessary for customizing the behavior of the file getter.
func UpdateGetters(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) func(*getter.Client) error {
	return func(client *getter.Client) error {
		// We copy all the default getters from the go-getter library, but replace the "file" getter. We shallow clone the
		// getter map here rather than using getter.Getters directly because (a) we shouldn't change the original,
		// globally-shared getter.Getters map and (b) Terragrunt may run this code from many goroutines concurrently during
		// xxx-all calls, so creating a new map each time ensures we don't a "concurrent map writes" error.
		client.Getters = map[string]getter.Getter{}

		for getterName, getterValue := range getter.Getters {
			if getterName == "file" {
				var includeInCopy, excludeFromCopy []string

				if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.IncludeInCopy != nil {
					includeInCopy = *terragruntConfig.Terraform.IncludeInCopy
				}

				if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.ExcludeFromCopy != nil {
					excludeFromCopy = *terragruntConfig.Terraform.ExcludeFromCopy
				}

				client.Getters[getterName] = &FileCopyGetter{
					IncludeInCopy:   includeInCopy,
					ExcludeFromCopy: excludeFromCopy,
				}
			} else {
				client.Getters[getterName] = getterValue
			}
		}

		// Load in custom getters that are only supported in Terragrunt
		client.Getters["tfr"] = &tf.RegistryGetter{
			TerragruntOptions: terragruntOptions,
		}

		return nil
	}
}

// preserveSymlinksOption is a custom client option that ensures DisableSymlinks
// setting is preserved during git operations
func preserveSymlinksOption() getter.ClientOption {
	return func(c *getter.Client) error {
		// Create a custom git getter that preserves symlink settings
		if c.Getters != nil {
			if gitGetter, exists := c.Getters["git"]; exists {
				// Replace with a wrapper that preserves symlink settings
				c.Getters["git"] = &symlinkPreservingGitGetter{
					original: gitGetter,
					client:   c,
				}
			}
		}

		// Ensure DisableSymlinks is set to false
		c.DisableSymlinks = false

		return nil
	}
}

// Download the code from the Canonical Source URL into the Download Folder using the go-getter library
func downloadSource(ctx context.Context, l log.Logger, src *tf.Source, opts *options.TerragruntOptions, cfg *config.TerragruntConfig, r *report.Report) error {
	canonicalSourceURL := src.CanonicalSourceURL.String()

	// Since we convert abs paths to rel in logs, `file://../../path/to/dir` doesn't look good,
	// so it's better to get rid of it.
	canonicalSourceURL = strings.TrimPrefix(canonicalSourceURL, fileURIScheme)

	l.Infof(
		"Downloading Terraform configurations from %s into %s",
		canonicalSourceURL,
		src.DownloadDir)

	allowCAS := opts.Experiments.Evaluate(experiment.CAS)

	isLocalSource := tf.IsLocalSource(src.CanonicalSourceURL)

	if allowCAS && !isLocalSource {
		l.Debugf("CAS experiment enabled: attempting to use Content Addressable Storage for source: %s", canonicalSourceURL)

		c, err := cas.New(cas.Options{})
		if err != nil {
			l.Warnf("Failed to initialize CAS: %v. Falling back to standard getter.", err)
		} else {
			cloneOpts := cas.CloneOptions{
				Dir:              src.DownloadDir,
				IncludedGitFiles: []string{"HEAD", "config"},
			}

			casGetter := cas.NewCASGetter(l, c, &cloneOpts)

			// Use go-getter v2 Client to properly process the Request
			client := getterv2.Client{
				Getters: []getterv2.Getter{casGetter},
			}

			// Set Pwd to the working directory so go-getter v2 can resolve relative paths
			req := &getterv2.Request{
				Src: src.CanonicalSourceURL.String(),
				Dst: src.DownloadDir,
				Pwd: opts.WorkingDir,
			}

			if _, casErr := client.Get(ctx, req); casErr == nil {
				l.Debugf("Successfully downloaded source using CAS: %s", canonicalSourceURL)
				return nil
			} else {
				l.Warnf("CAS download failed: %v. Falling back to standard getter.", casErr)
			}
		}
	}

	// Fallback to standard go-getter
	return opts.RunWithErrorHandling(ctx, l, r, func() error {
		return getter.GetAny(src.DownloadDir, src.CanonicalSourceURL.String(), UpdateGetters(opts, cfg), preserveSymlinksOption())
	})
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
