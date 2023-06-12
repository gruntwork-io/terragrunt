package terraform

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "*"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:   CommandName,
		Usage:  "Terragrunt forwards all other commands directly to Terraform",
		Before: func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
	}

	return command
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		opts.RunTerragrunt = Run

		if opts.TerraformCommand == CommandNameDestroy {
			opts.CheckDependentModules = true
		}

		return Run(opts)
	}
}
