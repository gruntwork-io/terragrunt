//go:build windows

package util

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// ensureTempDir uses %TEMP%\terragrunt. Windows gives each user a temp
// directory of their own, so the path needs no user id.
func ensureTempDir(v *venv.Venv) (string, error) {
	tempDir := filepath.Join(v.Platform.TempDir(), "terragrunt")

	if err := v.FS.MkdirAll(tempDir, os.ModePerm); err != nil {
		return "", err
	}

	return tempDir, nil
}
