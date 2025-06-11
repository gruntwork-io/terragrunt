package stack

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	stackDir = ".terragrunt-stack"
)

// RunGenerate runs the stack command.
func RunGenerate(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, config.DefaultStackFile)

	if opts.NoStackGenerate {
		l.Debugf("Skipping stack generation for %s", opts.TerragruntStackConfigPath)
		return nil
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		return config.GenerateStacks(ctx, l, opts)
	})
}

// Run execute stack command.
func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_run", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		return RunGenerate(ctx, l, opts)
	})

	if err != nil {
		return err
	}

	opts.WorkingDir = filepath.Join(opts.WorkingDir, stackDir)

	return runall.Run(ctx, l, opts)
}

// RunOutput stack output.
func RunOutput(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, index string) error {
	var outputs cty.Value

	// collect outputs
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_output", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		stackOutputs, err := config.StackOutput(ctx, l, opts)
		outputs = stackOutputs

		return err
	})
	if err != nil {
		return errors.New(err)
	}

	// Filter outputs based on index key
	filteredOutputs := FilterOutputs(outputs, index)

	// render outputs

	writer := opts.Writer

	switch opts.StackOutputFormat {
	default:
		if err := PrintOutputs(writer, filteredOutputs); err != nil {
			return errors.New(err)
		}

	case rawOutputFormat:
		if err := PrintRawOutputs(opts, writer, filteredOutputs); err != nil {
			return errors.New(err)
		}

	case jsonOutputFormat:
		if err := PrintJSONOutput(writer, filteredOutputs); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// FilterOutputs filters the outputs based on the provided index key.
func FilterOutputs(outputs cty.Value, index string) cty.Value {
	if !outputs.IsKnown() || outputs.IsNull() || len(index) == 0 {
		return outputs
	}

	// Split the index into parts
	indexParts := strings.Split(index, ".")
	// Traverse the map using the index parts
	currentValue := outputs
	for _, part := range indexParts {
		// Check if the current value is a map or object
		if currentValue.Type().IsObjectType() || currentValue.Type().IsMapType() {
			valueMap := currentValue.AsValueMap()
			if nextValue, exists := valueMap[part]; exists {
				currentValue = nextValue
			} else {
				// If any part of the index path is not found, return NilVal
				return cty.NilVal
			}
		} else {
			// If the current value is not a map or object, return NilVal
			return cty.NilVal
		}
	}

	// Reconstruct the nested map structure
	nested := currentValue
	for i := len(indexParts) - 1; i >= 0; i-- {
		nested = cty.ObjectVal(map[string]cty.Value{
			indexParts[i]: nested,
		})
	}

	return nested
}

// RunClean recursively removes all stack directories under the specified WorkingDir.
func RunClean(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	telemeter := telemetry.TelemeterFromContext(ctx)
	err := telemeter.Collect(ctx, "stack_clean", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		errs := &errors.MultiError{}

		walkFn := func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				l.Warnf("Error accessing path %s: %v", path, walkErr)

				errs = errs.Append(walkErr)

				return nil
			}

			if d.IsDir() && d.Name() == stackDir {
				relPath, relErr := filepath.Rel(opts.WorkingDir, path)
				if relErr != nil {
					relPath = path // fallback to absolute if error
				}

				l.Infof("Deleting stack directory: %s", relPath)

				if rmErr := os.RemoveAll(path); rmErr != nil {
					l.Errorf("Failed to delete stack directory %s: %v", relPath, rmErr)

					errs = errs.Append(rmErr)
				}

				return filepath.SkipDir
			}

			return nil
		}

		if walkErr := filepath.WalkDir(opts.WorkingDir, walkFn); walkErr != nil {
			errs = errs.Append(walkErr)
		}

		return errs.ErrorOrNil()
	})

	if err != nil {
		return errors.Errorf("failed to clean stack directories under %q: %w", opts.WorkingDir, err)
	}

	return nil
}
