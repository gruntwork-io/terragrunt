package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, cmdOpts *Options, args cli.Args) error {
	if len(args) == 0 {
		return errors.New("target command not specified")
	}

	target := run.NewTarget(run.TargetPointInitCommand, runTargetCommand(cmdOpts, args))

	opts.AutoInit = false

	return run.RunWithTarget(ctx, opts, target)
}

func runTargetCommand(cmdOpts *Options, args cli.Args) run.TargetCallbackType {
	return func(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
		var (
			command = args.CommandName()
			args    = args.Tail()
			dir     = opts.WorkingDir
		)

		if cmdOpts.InDownloadDir && util.FileExists(opts.DownloadDir) {
			dir = opts.DownloadDir
		}

		return run.RunActionWithHooks(ctx, command, opts, cfg, func(ctx context.Context) error {
			_, err := shell.RunShellCommandWithOutput(ctx, opts, dir, false, false, command, args...)

			return err
		})
	}
}
