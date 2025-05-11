// Package backend provides commands for interacting with remote backends.
package backend

import (
	// "github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "backend"

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdPrefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:  CommandName,
		Usage: "Interact with OpenTofu/Terraform backend infrastructure.",
		Subcommands: cli.Commands{
			bootstrap.NewCommand(opts, cmdPrefix),
			delete.NewCommand(opts, cmdPrefix),
			migrate.NewCommand(opts, cmdPrefix),
		},
		Action: cli.ShowCommandHelp,
	}
}
