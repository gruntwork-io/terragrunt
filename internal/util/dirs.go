package util

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// DefaultWorkingAndDownloadDirs gets the default working and download
// directories for the given Terragrunt config path.
func DefaultWorkingAndDownloadDirs(terragruntConfigPath string) (string, string, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	downloadDir, err := filepath.Abs(filepath.Join(workingDir, TerragruntCacheDir))
	if err != nil {
		return "", "", errors.New(err)
	}

	return workingDir, downloadDir, nil
}
