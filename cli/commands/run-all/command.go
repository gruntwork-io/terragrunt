package runall

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "run-all"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:        CommandName,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.",
		UsageText:   fmt.Sprintf("terragrunt %s <terraform command> [terraform options] [global options]", CommandName),
		Before:      func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action: func(ctx *cli.Context) error {
			opts.RunTerragrunt = terraform.Run

			return Run(opts)
		},
	}

	return command
}
