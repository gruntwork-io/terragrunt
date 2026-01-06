package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cmdOpts *Options, args cli.Args) error {
	targetConfigPoint := run.TargetPointInitCommand

	if !cmdOpts.InDownloadDir {
		targetConfigPoint = run.TargetPointSetInputsAsEnvVars
		opts.AutoInit = false
	}

	target := run.NewTarget(targetConfigPoint, runTargetCommand(cmdOpts, args))

	// exec command doesn't run terraform, so create a placeholder execution
	// The target callback will be invoked before terraform execution
	exec := &cli.TerraformExecution{
		Cmd:  "exec",
		Args: args.Tail(),
	}

	return run.RunWithTarget(ctx, l, opts, exec, report.NewReport(), target)
}

func runTargetCommand(cmdOpts *Options, args cli.Args) run.TargetCallbackType {
	return func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
		var (
			command = args.CommandName()
			args    = args.Tail()
			dir     = opts.WorkingDir
		)

		if !cmdOpts.InDownloadDir {
			dir = opts.RootWorkingDir
		}

		return run.RunActionWithHooks(ctx, l, command, opts, cfg, report.NewReport(), func(ctx context.Context) error {
			_, err := shell.RunCommandWithOutput(ctx, l, opts, dir, false, false, command, args...)
			if err != nil {
				return errors.Errorf("failed to run command in directory %s: %w", dir, err)
			}

			return nil
		})
	}
}
