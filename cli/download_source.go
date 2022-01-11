package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/cli/tfsource"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/internal/tfr"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// manifest for files copied from terragrunt module folder (i.e., the folder that contains the current terragrunt.hcl)
const MODULE_MANIFEST_NAME = ".terragrunt-module-manifest"

// file to indicate that init should be executed
const moduleInitRequiredFile = ".terragrunt-init-required"

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Check if module directory exists in temporary folder
// 3. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 4. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the NewTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformSource(source string, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (*options.TerragruntOptions, error) {
	terraformSource, err := tfsource.NewTerraformSource(source, terragruntOptions.DownloadDir, terragruntOptions.WorkingDir, terragruntOptions.Logger)
	if err != nil {
		return nil, err
	}

	if err := downloadTerraformSourceIfNecessary(terraformSource, terragruntOptions, terragruntConfig); err != nil {
		return nil, err
	}

	terragruntOptions.Logger.Debugf("Copying files from %s into %s", terragruntOptions.WorkingDir, terraformSource.WorkingDir)
	var includeInCopy []string
	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.IncludeInCopy != nil {
		includeInCopy = *terragruntConfig.Terraform.IncludeInCopy
	}
	if err := util.CopyFolderContents(terragruntOptions.WorkingDir, terraformSource.WorkingDir, MODULE_MANIFEST_NAME, includeInCopy); err != nil {
		return nil, err
	}

	updatedTerragruntOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)

	terragruntOptions.Logger.Debugf("Setting working directory to %s", terraformSource.WorkingDir)
	updatedTerragruntOptions.WorkingDir = terraformSource.WorkingDir

	return updatedTerragruntOptions, nil
}

// Download the specified TerraformSource if the latest code hasn't already been downloaded.
func downloadTerraformSourceIfNecessary(terraformSource *tfsource.TerraformSource, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntOptions.SourceUpdate {
		terragruntOptions.Logger.Debugf("The --%s flag is set, so deleting the temporary folder %s before downloading source.", optTerragruntSourceUpdate, terraformSource.DownloadDir)
		if err := os.RemoveAll(terraformSource.DownloadDir); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	alreadyLatest, err := alreadyHaveLatestCode(terraformSource, terragruntOptions)
	if err != nil {
		return err
	}

	if alreadyLatest {
		if err := validateWorkingDir(terraformSource); err != nil {
			return err
		}
		terragruntOptions.Logger.Debugf("Terraform files in %s are up to date. Will not download again.", terraformSource.WorkingDir)
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
	terragruntOptionsForDownload := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsForDownload.TerraformCommand = CMD_INIT_FROM_MODULE
	downloadErr := runActionWithHooks("download source", terragruntOptionsForDownload, terragruntConfig, func() error {
		return downloadSource(terraformSource, terragruntOptions, terragruntConfig)
	})

	if downloadErr != nil {
		return downloadErr
	}

	if err := terraformSource.WriteVersionFile(); err != nil {
		return err
	}

	if err := validateWorkingDir(terraformSource); err != nil {
		return err
	}

	currentVersion := terraformSource.EncodeSourceVersion()
	// if source versions are different, create file to run init
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	if previousVersion != currentVersion {
		initFile := util.JoinPath(terraformSource.WorkingDir, moduleInitRequiredFile)
		f, createErr := os.Create(initFile)
		if createErr != nil {
			return createErr
		}
		defer f.Close()
	}

	return nil
}

// Returns true if the specified TerraformSource, of the exact same version, has already been downloaded into the
// DownloadFolder. This helps avoid downloading the same code multiple times. Note that if the TerraformSource points
// to a local file path, we assume the user is doing local development and always return false to ensure the latest
// code is downloaded (or rather, copied) every single time. See the ProcessTerraformSource method for more info.
func alreadyHaveLatestCode(terraformSource *tfsource.TerraformSource, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if tfsource.IsLocalSource(terraformSource.CanonicalSourceURL) ||
		!util.FileExists(terraformSource.DownloadDir) ||
		!util.FileExists(terraformSource.WorkingDir) ||
		!util.FileExists(terraformSource.VersionFile) {

		return false, nil
	}

	tfFiles, err := filepath.Glob(fmt.Sprintf("%s/*.tf", terraformSource.WorkingDir))
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	if len(tfFiles) == 0 {
		terragruntOptions.Logger.Debugf("Working dir %s exists but contains no Terraform files, so assuming code needs to be downloaded again.", terraformSource.WorkingDir)
		return false, nil
	}

	currentVersion := terraformSource.EncodeSourceVersion()
	previousVersion, err := readVersionFile(terraformSource)

	if err != nil {
		return false, err
	}

	return previousVersion == currentVersion, nil
}

// Return the version number stored in the DownloadDir. This version number can be used to check if the Terraform code
// that has already been downloaded is the same as the version the user is currently requesting. The version number is
// calculated using the encodeSourceVersion method.
func readVersionFile(terraformSource *tfsource.TerraformSource) (string, error) {
	return util.ReadFileAsString(terraformSource.VersionFile)
}

// updateGetters returns the customized go-getter interfaces that Terragrunt relies on. Specifically:
// - Local file path getter is updated to copy the files instead of creating symlinks, which is what go-getter defaults
//   to.
// - Include the customized getter for fetching sources from the Terraform Registry.
// This creates a closure that returns a function so that we have access to the terragrunt configuration, which is
// necessary for customizing the behavior of the file getter.
func updateGetters(terragruntConfig *config.TerragruntConfig) func(*getter.Client) error {
	return func(client *getter.Client) error {
		// We copy all the default getters from the go-getter library, but replace the "file" getter. We shallow clone the
		// getter map here rather than using getter.Getters directly because (a) we shouldn't change the original,
		// globally-shared getter.Getters map and (b) Terragrunt may run this code from many goroutines concurrently during
		// xxx-all calls, so creating a new map each time ensures we don't a "concurrent map writes" error.
		client.Getters = map[string]getter.Getter{}
		for getterName, getterValue := range getter.Getters {
			if getterName == "file" {
				var includeInCopy []string
				if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.IncludeInCopy != nil {
					includeInCopy = *terragruntConfig.Terraform.IncludeInCopy
				}
				client.Getters[getterName] = &FileCopyGetter{IncludeInCopy: includeInCopy}
			} else {
				client.Getters[getterName] = getterValue
			}
		}

		// Load in custom getters that are only supported in Terragrunt
		client.Getters["tfr"] = &tfr.TerraformRegistryGetter{}

		return nil
	}
}

// Download the code from the Canonical Source URL into the Download Folder using the go-getter library
func downloadSource(terraformSource *tfsource.TerraformSource, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terragruntOptions.Logger.Debugf("Downloading Terraform configurations from %s into %s", terraformSource.CanonicalSourceURL, terraformSource.DownloadDir)

	if err := getter.GetAny(terraformSource.DownloadDir, terraformSource.CanonicalSourceURL.String(), updateGetters(terragruntConfig)); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

// Check if working terraformSource.WorkingDir exists and is directory
func validateWorkingDir(terraformSource *tfsource.TerraformSource) error {
	workingLocalDir := strings.Replace(terraformSource.WorkingDir, terraformSource.DownloadDir+filepath.FromSlash("/"), "", -1)
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
