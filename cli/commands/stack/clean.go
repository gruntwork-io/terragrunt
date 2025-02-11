package stack

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// RunClean cleans the stack directory
func RunClean(_ context.Context, opts *options.TerragruntOptions) error {
	baseDir := filepath.Join(opts.WorkingDir, stackDir)
	opts.Logger.Debugf("Cleaning stack directory: %s", baseDir)
	err := os.RemoveAll(baseDir)
	if err != nil {
		return errors.Errorf("failed to clean stack directory: %s %w", baseDir, err)
	}
	return nil
}
