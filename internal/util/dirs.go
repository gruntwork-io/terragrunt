package util

import (
	"path/filepath"
)

// DefaultWorkingAndDownloadDirs gets the default working and download
// directories for the given Terragrunt config path.
func DefaultWorkingAndDownloadDirs(terragruntConfigPath string) (string, string) {
	workingDir := filepath.Dir(terragruntConfigPath)

	downloadDir := filepath.Clean(filepath.Join(workingDir, TerragruntCacheDir))

	return workingDir, downloadDir
}
