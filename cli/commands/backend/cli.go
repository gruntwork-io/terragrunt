// Package backend provides commands for interacting with remote backends.
package backend

import (
	// "github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "backend"

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Interact with OpenTofu/Terraform backend infrastructure.",
		Subcommands: cli.Commands{
			bootstrap.NewCommand(opts),
			delete.NewCommand(opts),
			migrate.NewCommand(opts),
		},
		Action: cli.ShowCommandHelp,
	}
}
