package stack

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/stacks/clean"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/stacks/output"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// RunGenerate runs the stack command.
func RunGenerate(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, config.DefaultStackFile)

	if opts.NoStackGenerate {
		l.Debugf("Skipping stack generation for %s", opts.TerragruntStackConfigPath)
		return nil
	}

	tips.GiveStackTargetTip(l, v.FS, opts.WorkingDir, opts.Filters, opts.Tips)

	opts.StackAction = "generate"

	// Clean stack folders before calling `generate` when the `--source-update` flag is passed
	if opts.SourceUpdate {
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_clean", map[string]any{
			"stack_config_path": opts.TerragruntStackConfigPath,
			"working_dir":       opts.WorkingDir,
		}, func(ctx context.Context, l log.Logger) error {
			l.Debugf("Running stack clean for %s, as part of generate command", opts.WorkingDir)
			return clean.CleanStacks(l, v.FS, opts)
		})
		if err != nil {
			return fmt.Errorf(
				"failed to clean stack directories under %q: %w",
				opts.WorkingDir,
				err,
			)
		}
	}

	filters := opts.Filters

	gitFilters := filters.UniqueGitFilters()

	// Only create worktrees when git filter expressions are present
	var wts *worktrees.Worktrees

	if len(gitFilters) > 0 {
		var err error

		wts, err = worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
			WorkingDir:     opts.WorkingDir,
			GitExpressions: gitFilters,
			Experiments:    opts.Experiments,
		})
		if err != nil {
			return fmt.Errorf("failed to create worktrees: %w", err)
		}

		defer func() {
			cleanupErr := wts.Cleanup(ctx, l, v.FS)
			if cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	gen := generate.NewGenerator()

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_generate", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return gen.GenerateStacks(ctx, l, v, opts, wts)
	})
	if err != nil {
		return err
	}

	// After generation, hint when a literal stack filter left nested stacks ungenerated.
	funcsFor := configbridge.StackFuncFactory(ctx, l, v, opts)
	tips.GiveStackNestedGenerateTip(
		l,
		v.FS,
		funcsFor,
		opts.WorkingDir,
		opts.Filters,
		opts.Tips,
	)

	return nil
}

// Run executes the stack command.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	opts.StackAction = "run"

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_run", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return RunGenerate(ctx, l, v, opts)
	})
	if err != nil {
		return err
	}

	return runall.Run(ctx, l, v, opts)
}

// RunOutput stack output.
func RunOutput(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	index string,
) error {
	opts.StackAction = "output"
	opts.TerraformCommand = "output" // required for discovery exclude action matching in StackOutput

	var outputs cty.Value

	// collect outputs
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_output", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		stackOutputs, err := output.StackOutput(ctx, l, v, opts)
		outputs = stackOutputs

		return err
	})
	if err != nil {
		return err
	}

	filteredOutputs, err := FilterOutputs(outputs, index)
	if err != nil {
		return err
	}

	// render outputs

	switch opts.StackOutputFormat {
	default:
		if err := PrintOutputs(v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}

	case rawOutputFormat:
		if err := PrintRawOutputs(opts, v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}

	case jsonOutputFormat:
		if err := PrintJSONOutput(v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}
	}

	return nil
}

// FilterOutputs narrows outputs to the value at index, an address such as `vpc.id`,
// `shard["web"].id`, or `shard[0].id`, still wrapped in the objects it was nested in.
// An address that names nothing yields [cty.NilVal], which prints as no output.
func FilterOutputs(outputs cty.Value, index string) (cty.Value, error) {
	if !outputs.IsKnown() || outputs.IsNull() || len(index) == 0 {
		return outputs, nil
	}

	segments, err := parseOutputAddress(index)
	if err != nil {
		return cty.NilVal, err
	}

	current := outputs

	for _, segment := range segments {
		if !current.Type().IsObjectType() && !current.Type().IsMapType() {
			return cty.NilVal, nil
		}

		next, exists := current.AsValueMap()[segment]
		if !exists {
			return cty.NilVal, nil
		}

		current = next
	}

	for i := len(segments) - 1; i >= 0; i-- {
		current = cty.ObjectVal(map[string]cty.Value{segments[i]: current})
	}

	return current, nil
}

// parseOutputAddress splits an output address into the segments it names. It reads the
// address as an HCL traversal rather than splitting on dots, so that one element of an
// expanded unit can be addressed as `shard["web"]` and an iteration key containing a dot
// stays in one piece.
func parseOutputAddress(index string) ([]string, error) {
	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(index), "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, InvalidOutputAddressError{Address: index, Err: diags}
	}

	segments := make([]string, 0, len(traversal))

	for _, step := range traversal {
		switch step := step.(type) {
		case hcl.TraverseRoot:
			segments = append(segments, step.Name)
		case hcl.TraverseAttr:
			segments = append(segments, step.Name)
		case hcl.TraverseIndex:
			key, err := convert.Convert(step.Key, cty.String)
			if err != nil {
				return nil, InvalidOutputAddressError{Address: index, Err: err}
			}

			segments = append(segments, key.AsString())
		default:
			return nil, InvalidOutputAddressError{
				Address: index,
				Err:     fmt.Errorf("unsupported address step %T", step),
			}
		}
	}

	return segments, nil
}

// InvalidOutputAddressError reports an output address that cannot be parsed. An address
// that parses but names no unit is not an error; it yields no output.
type InvalidOutputAddressError struct {
	Err     error
	Address string
}

func (err InvalidOutputAddressError) Error() string {
	return fmt.Sprintf("invalid output address %q: %v", err.Address, err.Err)
}

func (err InvalidOutputAddressError) Unwrap() error {
	return err.Err
}

// RunClean recursively removes all stack directories under the specified WorkingDir.
func RunClean(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	telemeter := telemetry.TelemeterFromContext(ctx)

	err := telemeter.Collect(ctx, l, "stack_clean", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return clean.CleanStacks(l, v.FS, opts)
	})
	if err != nil {
		return fmt.Errorf("failed to clean stack directories under %q: %w", opts.WorkingDir, err)
	}

	return nil
}
