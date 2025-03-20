package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, cmdOpts *Options, args cli.Args) error {
	targetConfigPoint := run.TargetPointInitCommand

	if !cmdOpts.InDownloadDir {
		targetConfigPoint = run.TargetPointSetInputsAsEnvVars
		opts.AutoInit = false
	}

	target := run.NewTarget(targetConfigPoint, runTargetCommand(cmdOpts, args))

	return run.RunWithTarget(ctx, opts, target)
}

func runTargetCommand(cmdOpts *Options, args cli.Args) run.TargetCallbackType {
	return func(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
		var (
			command  = args.CommandName()
			tailArgs = args.Tail()
			dir      = opts.WorkingDir
		)

		if !cmdOpts.InDownloadDir {
			dir = opts.RootWorkingDir
		}

		return run.RunActionWithHooks(ctx, command, opts, cfg, func(ctx context.Context) error {
			_, err := shell.RunCommandWithOutput(ctx, opts, dir, false, false, command, tailArgs...)
			if err != nil {
				return errors.Errorf("failed to run command in directory %s: %w", dir, err)
			}

			return nil
		})
	}
}
