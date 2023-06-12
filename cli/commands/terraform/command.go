package terraform

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "*"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:  CommandName,
		Usage: "Terragrunt forwards all other commands directly to Terraform",
	}

	return command
}

func CommandAction(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		opts.RunTerragrunt = Run

		if err := common.InitialSetup(ctx, opts); err != nil {
			return err
		}

		return Run(opts)
	}
}
