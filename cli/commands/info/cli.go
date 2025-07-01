// Package info represents list of info commands that display various Terragrunt settings.
package info

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "info"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "List of commands to display Terragrunt settings.",
		Subcommands: cli.Commands{
			strict.NewCommand(l, opts),
			print.NewCommand(l, opts),
		},
		Action: cli.ShowCommandHelp,
	}
}
