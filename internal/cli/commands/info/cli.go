// Package info represents list of info commands that display various Terragrunt settings.
package info

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt info` command.
	CommandName = "info"
)

// NewCommand returns the `terragrunt info` command, which groups the strict
// and print subcommands and prints help when run alone.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "List of commands to display Terragrunt settings.",
		Subcommands: clihelper.Commands{
			strict.NewCommand(l, opts),
			print.NewCommand(l, opts, v),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
