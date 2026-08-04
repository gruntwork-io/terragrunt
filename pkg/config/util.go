package config

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// CopyLockFile copies the lock file from the source folder to the destination folder.
//
// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder
func CopyLockFile(
	l log.Logger,
	fsys vfs.FS,
	rootWorkingDir string,
	logShowAbsPaths bool,
	sourceFolder, destinationFolder string,
) error {
	sourceLockFilePath := filepath.Join(sourceFolder, tf.TerraformLockFile)
	destinationLockFilePath := filepath.Join(destinationFolder, tf.TerraformLockFile)

	exists, err := vfs.FileExists(fsys, sourceLockFilePath)
	if err != nil {
		return err
	}

	if exists {
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

		return vfs.CopyFile(fsys, sourceLockFilePath, destinationLockFilePath)
	}

	return nil
}
