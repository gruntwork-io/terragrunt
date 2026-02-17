package config

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// CopyLockFile copies the lock file from the source folder to the destination folder.
//
// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder
func CopyLockFile(l log.Logger, rootWorkingDir string, logShowAbsPaths bool, sourceFolder, destinationFolder string) error {
	sourceLockFilePath := filepath.Join(sourceFolder, tf.TerraformLockFile)
	destinationLockFilePath := filepath.Join(destinationFolder, tf.TerraformLockFile)

	if util.FileExists(sourceLockFilePath) {
		l.Debugf(
			"Copying lock file from %s to %s",
			util.RelPathForLog(
				rootWorkingDir,
				sourceLockFilePath,
				logShowAbsPaths,
			),
			util.RelPathForLog(
				rootWorkingDir,
				destinationLockFilePath,
				logShowAbsPaths,
			),
		)

		return util.CopyFile(sourceLockFilePath, destinationLockFilePath)
	}

	return nil
}
