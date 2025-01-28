package stack

import (
	"context"

	"path/filepath"
	"strings"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"

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
	stacksEnabled := opts.Experiments[experiment.Stacks]
	if !stacksEnabled.Enabled {
		return errors.New("stacks experiment is not enabled use --experiment stacks to enable it")
	}

	return generateStack(ctx, opts)
}

// Run execute stack command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	stacksEnabled := opts.Experiments[experiment.Stacks]
	if !stacksEnabled.Enabled {
		return errors.New("stacks experiment is not enabled use --experiment stacks to enable it")
	}

	if err := RunGenerate(ctx, opts); err != nil {
		return err
	}

	// prepare options for execution
	// navigate to stack directory
	opts.WorkingDir = filepath.Join(opts.WorkingDir, stackDir)
	// remove 0 element from args
	opts.TerraformCliArgs = opts.TerraformCliArgs[1:]
	opts.TerraformCommand = opts.TerraformCliArgs[0]
	opts.OriginalTerraformCommand = strings.Join(opts.TerraformCliArgs, " ")

	return runall.Run(ctx, opts)
}

// RunOutput stack output.
func RunOutput(ctx context.Context, opts *options.TerragruntOptions) error {

	stacksEnabled := opts.Experiments[experiment.Stacks]
	if !stacksEnabled.Enabled {
		return errors.New("stacks experiment is not enabled use --experiment stacks to enable it")
	}

	return generateOutput(ctx, opts)
}
