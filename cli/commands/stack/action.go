package stack

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/zclconf/go-cty/cty"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/internal/cli"

	"github.com/gruntwork-io/terragrunt/internal/experiment"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	stackDir         = ".terragrunt-stack"
	defaultStackFile = "terragrunt.stack.hcl"
	dirPerm          = 0755
)

// RunGenerate runs the stack command.
func RunGenerate(ctx context.Context, opts *options.TerragruntOptions) error {
	if err := checkStackExperiment(opts); err != nil {
		return err
	}

	return generateStack(ctx, opts)
}

// Run execute stack command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	if err := checkStackExperiment(opts); err != nil {
		return err
	}

	if err := RunGenerate(ctx, opts); err != nil {
		return err
	}

	// read stack file and prepare values
	stackFile, err := config.ReadStackConfigFile(ctx, opts)
	if err != nil {
		return errors.New(err)
	}
	unitValues := map[string]*cty.Value{}
	for _, unit := range stackFile.Units {
		path := filepath.Join(opts.WorkingDir, stackDir, unit.Path)
		unitValues[path] = unit.Values
	}
	opts.UnitValues = unitValues

	opts.WorkingDir = filepath.Join(opts.WorkingDir, stackDir)

	return runall.Run(ctx, opts)
}

// RunOutput stack output.
func RunOutput(ctx context.Context, opts *options.TerragruntOptions, index string) error {
	if err := checkStackExperiment(opts); err != nil {
		return err
	}

	// collect outputs
	outputs, err := generateOutput(ctx, opts)
	if err != nil {
		return errors.New(err)
	}
	// write outputs

	writer := opts.Writer

	switch opts.StackOutputFormat {
	default:
		if err := PrintOutputs(writer, outputs, index); err != nil {
			return errors.New(err)
		}

	case rawOutputFormat:
		if err := PrintRawOutputs(opts, writer, outputs, index); err != nil {
			return errors.New(err)
		}

	case jsonOutputFormat:
		if err := PrintJSONOutput(writer, outputs, index); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// RunClean cleans the stack directory
func RunClean(_ context.Context, opts *options.TerragruntOptions) error {
	if err := checkStackExperiment(opts); err != nil {
		return err
	}

	baseDir := filepath.Join(opts.WorkingDir, stackDir)
	opts.Logger.Debugf("Cleaning stack directory: %s", baseDir)
	err := os.RemoveAll(baseDir)

	if err != nil {
		return errors.Errorf("failed to clean stack directory: %s %w", baseDir, err)
	}

	return nil
}

func checkStackExperiment(opts *options.TerragruntOptions) error {
	if !opts.Experiments.Evaluate(experiment.Stacks) {
		return cli.NewExitError(errors.New("stacks experiment is not enabled use --experiment stacks to enable it"), cli.ExitCodeGeneralError)
	}

	return nil
}
