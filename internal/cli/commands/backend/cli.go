// Package backend provides commands for interacting with remote backends.
package backend

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CommandName is the name of the `terragrunt backend` command.
const CommandName = "backend"

// NewCommand returns the `terragrunt backend` command, which groups the
// bootstrap, delete, and migrate subcommands and prints help when run alone.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Interact with OpenTofu/Terraform backend infrastructure.",
		Subcommands: clihelper.Commands{
			bootstrap.NewCommand(l, opts, v),
			delete.NewCommand(l, opts, v),
			migrate.NewCommand(l, opts, v),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
