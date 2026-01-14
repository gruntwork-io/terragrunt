// Package info represents list of info commands that display various Terragrunt settings.
package info

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "info"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "List of commands to display Terragrunt settings.",
		Subcommands: clihelper.Commands{
			strict.NewCommand(l, opts),
			print.NewCommand(l, opts),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
