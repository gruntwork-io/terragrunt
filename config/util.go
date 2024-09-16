package config

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
)

// CopyLockFile copies the lock file from the source folder to the destination folder.
//
// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder
func CopyLockFile(opts *options.TerragruntOptions, sourceFolder, destinationFolder string) error {
	sourceLockFilePath := util.JoinPath(sourceFolder, terraform.TerraformLockFile)
	destinationLockFilePath := util.JoinPath(destinationFolder, terraform.TerraformLockFile)

	if util.FileExists(sourceLockFilePath) {
		opts.Logger.Debugf("Copying lock file from %s to %s", sourceLockFilePath, destinationFolder)
		return util.CopyFile(sourceLockFilePath, destinationLockFilePath)
	}

	return nil
}
