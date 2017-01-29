package cli

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/config"
	"os"
	"github.com/gruntwork-io/terragrunt/errors"
	"path/filepath"
	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"
	"encoding/base64"
)

type TerraformSource struct {
	// The source path specified by the user
	RawSource          string

	// The canonical URL we compute from the source path
	CanonicalSourceUrl string

	// The folder where we should download the source to
	DownloadFolder     string

	// True if the source path points to a local file path and false otherwise
	IsLocalSource      bool
}

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 3. Set terragruntOptions.WorkingDir to the temporary folder.
func downloadTerraformSource(source string, terragruntOptions *options.TerragruntOptions) error {
	terraformSource, err := processTerraformSource(source, terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Using temporary folder %s for Terraform files from source %s", terraformSource.DownloadFolder, terraformSource.CanonicalSourceUrl)

	// If the temp folder already exists, we assume we've already downloaded the Terraform files and don't need
	// to spend time doing it again. The only exception is if the source URL points to a local file path, in which
	// case the user is probably doing local, iterative development and we should assume the Terraform files have
	// changed and need to be recopied every time (which is very fast anyway).
	if !util.FileExists(terraformSource.DownloadFolder) {
		if err := terraformInit(source, terraformSource.DownloadFolder, terragruntOptions); err != nil {
			return err
		}
	} else if terraformSource.IsLocalSource {
		if err := cleanupTerraformFiles(terraformSource.DownloadFolder, terragruntOptions); err != nil {
			return err
		}
		if err := terraformInit(source, terraformSource.DownloadFolder, terragruntOptions); err != nil {
			return err
		}
	} else {
		terragruntOptions.Logger.Printf("Terraform files in %s are already up to date. Will not download again.", terraformSource.DownloadFolder)
	}
	
	terragruntOptions.Logger.Printf("Copying files from %s into %s", terragruntOptions.WorkingDir, terraformSource.DownloadFolder)
	if err := util.CopyFolderContents(terragruntOptions.WorkingDir, terraformSource.DownloadFolder); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Setting working directory to %s", terraformSource.DownloadFolder)
	terragruntOptions.WorkingDir = terraformSource.DownloadFolder

	return nil
}

// Take the given source path and create a TerraformSource struct from it, including the folder where the source should
// be downloaded to. We try to use the same download folder for a given source path by converting the source path to
// a canonical form (e.g. converting relative file paths to absolute file paths) and using the base 64 encoded version
// of the canonical path, within the OS tmp folder, as the download path. This way, if the source path doesn't change,
// we don't have to unnecessarily re-download the source code, modules, and Terraform state.
func processTerraformSource(source string, terragruntOptions *options.TerragruntOptions) (*TerraformSource, error) {
	canonicalUrl, err := getter.Detect(source, terragruntOptions.WorkingDir, getter.Detectors)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	sourceUrl, err := urlhelper.Parse(canonicalUrl)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	isLocalSource := sourceUrl.Scheme == "file"

	if isLocalSource {
		// Always use the canonical file path to ensure that a given path on the local file system is always
		// represented the same way (i.e. doesn't differ just because the user provided a different relative
		// path)
		canonicalUrl, err = util.CanonicalPath(sourceUrl.Path, "")
		if err != nil {
			return nil, err
		}
	}

	canonicalUrlHash := base64.StdEncoding.EncodeToString([]byte(canonicalUrl))
	downloadFolder := filepath.Join(os.TempDir(), "terragrunt-download", canonicalUrlHash)

	return &TerraformSource{
		RawSource: source,
		CanonicalSourceUrl: canonicalUrl,
		DownloadFolder: downloadFolder,
		IsLocalSource: isLocalSource,
	}, nil
}

// If this temp folder already exists, simply delete all the Terraform configurations (*.tf) within it
// (the terraform init command will redownload the latest ones), but leave all the other files, such
// as the .terraform folder with the downloaded modules and remote state settings.
func cleanupTerraformFiles(path string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Cleaning up existing *.tf files in %s", path)

	files, err := filepath.Glob(filepath.Join(path, "*.tf"))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return util.DeleteFiles(files)
}

// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the .terragrunt config file. If the user used one of these, this
// method returns the source URL and the boolean true; if not, this method returns an empty string and false.
func getTerraformSourceUrl(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (string, bool) {
	if terragruntOptions.Source != "" {
		return terragruntOptions.Source, true
	} else if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != "" {
		return terragruntConfig.Terraform.Source, true
	} else {
		return "", false
	}
}

// Download the code from source into dest using the terraform init command
func terraformInit(source string, dest string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Downloading Terraform configurations from %s into %s", source, dest)

	terragruntInitOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntInitOptions.TerraformCliArgs = []string{"init", source, dest}

	return runTerraformCommand(terragruntInitOptions)
}
