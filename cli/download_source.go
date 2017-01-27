package cli

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/config"
	"os"
	"github.com/gruntwork-io/terragrunt/errors"
	"path/filepath"
)

// 1. Check out the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 3. Set terragruntOptions.WorkingDir to the temporary folder.
func downloadTerraformSource(source string, terragruntOptions *options.TerragruntOptions) error {
	tmpFolder, err := prepareTempDir(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Downloading Terraform configurations from %s into %s", source, tmpFolder)
	if err := terraformInit(source, tmpFolder, terragruntOptions); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Copying files from %s into %s", terragruntOptions.WorkingDir, tmpFolder)
	if err := util.CopyFolderContents(terragruntOptions.WorkingDir, tmpFolder); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Setting working directory to %s", tmpFolder)
	terragruntOptions.WorkingDir = tmpFolder

	return nil
}

// Prepare a temp folder into which Terragrunt can download Terraform code. We take the canonical path of the current
// working directory and create a folder under the system temporary folder with the same path. This allows us to reuse
// the same temp folder for a given working directory so that if you run Terragrunt from the same folder multiple
// times, we only have to download modules and configure remote state once (however, we download the source code every
// time).
func prepareTempDir(terragruntOptions *options.TerragruntOptions) (string, error) {
	canonicalPath, err := util.CanonicalPath(terragruntOptions.WorkingDir, "")
	if err != nil {
		return "", err
	}

	tmpFolder := filepath.Join(os.TempDir(), "terragrunt-downloads", canonicalPath)

	if util.FileExists(tmpFolder) {
		terragruntOptions.Logger.Printf("Temp folder %s already exists. Will delete Terraform configurations within it before downloading the latest ones.", tmpFolder)
		if err := cleanupTerraformFiles(tmpFolder); err != nil {
			return "", err
		}
	} else {
		terragruntOptions.Logger.Printf("Creating temp folder %s to store downloaded Terraform configurations.", tmpFolder)
		if err := os.MkdirAll(tmpFolder, 0777); err != nil {
			return "", errors.WithStackTrace(err)
		}
	}

	return tmpFolder, nil
}

// If this temp folder already exists, simply delete all the Terraform configurations (*.tf) within it
// (the terraform init command will redownload the latest ones), but leave all the other files, such
// as the .terraform folder with the downloaded modules and remote state settings.
func cleanupTerraformFiles(path string) error {
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
	terragruntInitOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntInitOptions.TerraformCliArgs = []string{"init", source, dest}

	return runTerraformCommand(terragruntInitOptions)
}
