// Package backend provides commands for interacting with remote backends.
package backend

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const CommandName = "backend"

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Interact with OpenTofu/Terraform backend infrastructure.",
		Subcommands: clihelper.Commands{
			bootstrap.NewCommand(l, opts),
			delete.NewCommand(l, opts),
			migrate.NewCommand(l, opts),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
