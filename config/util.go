package config

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
)

// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder
func CopyLockFile(opts *options.TerragruntOptions, destinationFolder string) error {
	sourceFolder := filepath.Dir(opts.RelativeTerragruntConfigPath)
	sourceLockFilePath := util.JoinPath(sourceFolder, terraform.TerraformLockFile)
	destinationLockFilePath := util.JoinPath(destinationFolder, terraform.TerraformLockFile)

	relDestinationFolder, err := util.GetPathRelativeTo(destinationFolder, opts.RootWorkingDir)
	if err != nil {
		return err
	}

	if util.FileExists(sourceLockFilePath) {
		opts.Logger.Debugf("Copying lock file from %s to %s", sourceLockFilePath, relDestinationFolder)
		return util.CopyFile(sourceLockFilePath, destinationLockFilePath)
	}
	return nil
}
