package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, cmdOpts *Options, args cli.Args) error {
	if len(args) == 0 {
		return errors.New("exec command not specified")
	}

	target := terraform.NewTarget(terraform.TargetPointInitCommand, runTargetCommand(args))

	opts.AutoInit = false

	return terraform.RunWithTarget(ctx, opts, target)
}

func runTargetCommand(args cli.Args) terraform.TargetCallbackType {
	return func(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
		command := args.CommandName()
		args := args.Tail()

		return terraform.RunActionWithHooks(ctx, command, opts, cfg, func(ctx context.Context) error {
			return shell.RunShellCommand(ctx, opts, command, args...)
		})
	}
}
