package common

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// EnsureAbsolutePath ensures a path is absolute, converting it if necessary.
// Returns the absolute path and any error encountered during conversion.
func EnsureAbsolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	return absPath, nil
}
