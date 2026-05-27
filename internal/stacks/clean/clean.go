// Package clean provides the logic for cleaning up stack configurations.
package clean

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CleanStacks removes stack directories within the specified working directory, unless the command is "destroy".
// It returns an error if any issues occur during the deletion process, or nil if successful.
func CleanStacks(l log.Logger, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == tf.CommandNameDestroy {
		l.Debugf("Skipping stack clean for %s, as part of delete command", opts.WorkingDir)
		return nil
	}

	var errs []error

	walkFn := func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			l.Warnf("Error accessing path %s: %v", path, walkErr)

			errs = append(errs, walkErr)

			return nil
		}

		if d.IsDir() && d.Name() == ".terragrunt-stack" {
			relPath, relErr := filepath.Rel(opts.WorkingDir, path)
			if relErr != nil {
				relPath = path // fallback to absolute if error
			}

			l.Infof("Deleting stack directory: %s", relPath)

			if rmErr := os.RemoveAll(path); rmErr != nil {
				l.Errorf("Failed to delete stack directory %s: %v", relPath, rmErr)

				errs = append(errs, rmErr)
			}

			return filepath.SkipDir
		}

		return nil
	}
	if walkErr := filepath.WalkDir(opts.WorkingDir, walkFn); walkErr != nil {
		errs = append(errs, walkErr)
	}

	return errors.Join(errs...)
}
