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
		if err := terraformInit(terraformSource, terragruntOptions); err != nil {
			return err
		}
	} else if terraformSource.IsLocalSource {
		if err := cleanupTerraformFiles(terraformSource.DownloadFolder, terragruntOptions); err != nil {
			return err
		}
		if err := terraformInit(terraformSource, terragruntOptions); err != nil {
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
// be downloaded to. We try to use the same download folder, D, for a given source path by:
//
// 1. C = Convert the source path to a canonical form (e.g. converting relative file paths to absolute file paths)
// 2. H = Compute the base 64 encoded sha1 hash of C
// 3. D = TMP/terragrunt-download/H (where TMP is the temp folder for the current OS)
//
// By reusing the same folder, we only have to download the Terraform code, modules, and Terraform state for each
// source path once.
func processTerraformSource(source string, terragruntOptions *options.TerragruntOptions) (*TerraformSource, error) {
	workingDirAbs, err := filepath.Abs(terragruntOptions.WorkingDir)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	canonicalUrl, err := getter.Detect(source, workingDirAbs, getter.Detectors)
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

	downloadFolder := filepath.Join(os.TempDir(), "terragrunt-download", util.Base64EncodedSha1(canonicalUrl))

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

// Download the code from the Canonical Source URL into the Download Folder using the terraform init command
func terraformInit(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Downloading Terraform configurations from %s into %s", terraformSource.CanonicalSourceUrl, terraformSource.DownloadFolder)

	terragruntInitOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntInitOptions.TerraformCliArgs = []string{"init", terraformSource.CanonicalSourceUrl, terraformSource.DownloadFolder}

	return runTerraformCommand(terragruntInitOptions)
}
