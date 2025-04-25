// Package info represents list of info commands that display various Terragrunt settings.
package info

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "info"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return &cli.Command{
		Name:  CommandName,
		Usage: "List of commands to display Terragrunt settings.",
		Subcommands: cli.Commands{
			strict.NewCommand(opts, tgPrefix),
			print.NewCommand(opts, tgPrefix),
		},
		ErrorOnUndefinedFlag: true,
		Action:               cli.ShowCommandHelp,
	}
}
