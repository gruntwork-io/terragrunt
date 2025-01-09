package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
)

// ModuleManifestName is the manifest for files copied from terragrunt module folder (i.e., the folder that contains the current terragrunt.hcl).
const ModuleManifestName = ".terragrunt-module-manifest"

// ModuleInitRequiredFile is a file to indicate that init should be executed.
const ModuleInitRequiredFile = ".terragrunt-init-required"

const tfLintConfig = ".tflint.hcl"

const fileURIScheme = "file://"

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Check if module directory exists in temporary folder
// 3. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 4. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the NewTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformSource(ctx context.Context, source string, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (*options.TerragruntOptions, error) {
	experiment := opts.Experiments[experiment.Symlinks]
	walkWithSymlinks := experiment.Evaluate(opts.ExperimentMode)

	terraformSource, err := terraform.NewSource(source, opts.DownloadDir, opts.WorkingDir, opts.Logger, walkWithSymlinks)
	if err != nil {
		return nil, err
	}

	if err := DownloadTerraformSourceIfNecessary(ctx, terraformSource, opts, terragruntConfig); err != nil {
		return nil, err
	}

	opts.Logger.Debugf("Copying files from %s into %s", opts.WorkingDir, terraformSource.WorkingDir)

	var includeInCopy, excludeFromCopy []string

	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.IncludeInCopy != nil {
		includeInCopy = *terragruntConfig.Terraform.IncludeInCopy
	}

	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.ExcludeFromCopy != nil {
		excludeFromCopy = *terragruntConfig.Terraform.ExcludeFromCopy
	}

	// Always include the .tflint.hcl file, if it exists
	includeInCopy = append(includeInCopy, tfLintConfig)

	err = util.CopyFolderContents(
		opts.Logger,
		opts.WorkingDir,
		terraformSource.WorkingDir,
		ModuleManifestName,
		includeInCopy,
		excludeFromCopy,
	)
	if err != nil {
		return nil, err
	}

	updatedTerragruntOptions, err := opts.Clone(opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	opts.Logger.Debugf("Setting working directory to %s", terraformSource.WorkingDir)
	updatedTerragruntOptions.WorkingDir = terraformSource.WorkingDir

	return updatedTerragruntOptions, nil
}

// DownloadTerraformSourceIfNecessary downloads the specified TerraformSource if the latest code hasn't already been downloaded.
func DownloadTerraformSourceIfNecessary(ctx context.Context, terraformSource *terraform.Source, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntOptions.SourceUpdate {
		terragruntOptions.Logger.Debugf("The --%s flag is set, so deleting the temporary folder %s before downloading source.", commands.TerragruntSourceUpdateFlagName, terraformSource.DownloadDir)

		if err := os.RemoveAll(terraformSource.DownloadDir); err != nil {
			return errors.New(err)
		}
	}

	alreadyLatest, err := AlreadyHaveLatestCode(terraformSource, terragruntOptions)
	if err != nil {
		return err
	}

	if alreadyLatest {
		if err := ValidateWorkingDir(terraformSource); err != nil {
			return err
		}

		terragruntOptions.Logger.Debugf("%s files in %s are up to date. Will not download again.", terragruntOptions.TerraformImplementation, terraformSource.WorkingDir)

		return nil
	}

	var previousVersion = ""
	// read previous source version
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if util.FileExists(terraformSource.VersionFile) {
		previousVersion, err = readVersionFile(terraformSource)
		if err != nil {
			return err
		}
	}

	// When downloading source, we need to process any hooks waiting on `init-from-module`. Therefore, we clone the
	// options struct, set the command to the value the hooks are expecting, and run the download action surrounded by
	// before and after hooks (if any).
	terragruntOptionsForDownload, err := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	terragruntOptionsForDownload.TerraformCommand = terraform.CommandNameInitFromModule
	downloadErr := runActionWithHooks(ctx, "download source", terragruntOptionsForDownload, terragruntConfig, func(ctx context.Context) error {
		return downloadSource(terraformSource, terragruntOptions, terragruntConfig)
	})

	if downloadErr != nil {
		return DownloadingTerraformSourceErr{ErrMsg: downloadErr, URL: terraformSource.CanonicalSourceURL.String()}
	}

	if err := terraformSource.WriteVersionFile(); err != nil {
		return err
	}

	if err := ValidateWorkingDir(terraformSource); err != nil {
		return err
	}

	currentVersion, err := terraformSource.EncodeSourceVersion()
	// if source versions are different or calculating version failed, create file to run init
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if (previousVersion != "" && previousVersion != currentVersion) || err != nil {
		terragruntOptions.Logger.Debugf("Requesting re-init, source version has changed from %s to %s recently.", previousVersion, currentVersion)

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
func AlreadyHaveLatestCode(terraformSource *terraform.Source, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if !util.FileExists(terraformSource.DownloadDir) ||
		!util.FileExists(terraformSource.WorkingDir) ||
		!util.FileExists(terraformSource.VersionFile) {
		return false, nil
	}

	tfFiles, err := filepath.Glob(terraformSource.WorkingDir + "/*.tf")
	if err != nil {
		return false, errors.New(err)
	}

	if len(tfFiles) == 0 {
		terragruntOptions.Logger.Debugf("Working dir %s exists but contains no Terraform files, so assuming code needs to be downloaded again.", terraformSource.WorkingDir)
		return false, nil
	}

	currentVersion, err := terraformSource.EncodeSourceVersion()
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
func readVersionFile(terraformSource *terraform.Source) (string, error) {
	return util.ReadFileAsString(terraformSource.VersionFile)
}

// updateGetters returns the customized go-getter interfaces that Terragrunt relies on. Specifically:
//   - Local file path getter is updated to copy the files instead of creating symlinks, which is what go-getter defaults
//     to.
//   - Include the customized getter for fetching sources from the Terraform Registry.
//
// This creates a closure that returns a function so that we have access to the terragrunt configuration, which is
// necessary for customizing the behavior of the file getter.
func updateGetters(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) func(*getter.Client) error {
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
					includeInCopy = *terragruntConfig.Terraform.ExcludeFromCopy
				}

				client.Getters[getterName] = &FileCopyGetter{
					IncludeInCopy:   includeInCopy,
					Logger:          terragruntOptions.Logger,
					ExcludeFromCopy: excludeFromCopy,
				}
			} else {
				client.Getters[getterName] = getterValue
			}
		}

		// Load in custom getters that are only supported in Terragrunt
		client.Getters["tfr"] = &terraform.RegistryGetter{
			TerragruntOptions: terragruntOptions,
		}

		return nil
	}
}

// Download the code from the Canonical Source URL into the Download Folder using the go-getter library
func downloadSource(terraformSource *terraform.Source, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	canonicalSourceURL := terraformSource.CanonicalSourceURL.String()

	// Since we convert abs paths to rel in logs, `file://../../path/to/dir` doesn't look good,
	// so it's better to get rid of it.
	canonicalSourceURL = strings.TrimPrefix(canonicalSourceURL, fileURIScheme)

	terragruntOptions.Logger.Infof(
		"Downloading Terraform configurations from %s into %s",
		canonicalSourceURL,
		terraformSource.DownloadDir)

	if err := getter.GetAny(terraformSource.DownloadDir, terraformSource.CanonicalSourceURL.String(), updateGetters(terragruntOptions, terragruntConfig)); err != nil {
		return errors.New(err)
	}

	return nil
}

// ValidateWorkingDir checks if working terraformSource.WorkingDir exists and is directory
func ValidateWorkingDir(terraformSource *terraform.Source) error {
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
