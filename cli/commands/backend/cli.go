// Package backend provides commands for interacting with remote backends.
package backend

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const CommandName = "backend"

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Interact with OpenTofu/Terraform backend infrastructure.",
		Subcommands: cli.Commands{
			bootstrap.NewCommand(l, opts),
			delete.NewCommand(l, opts),
			migrate.NewCommand(l, opts),
		},
		Action: cli.ShowCommandHelp,
	}
}
