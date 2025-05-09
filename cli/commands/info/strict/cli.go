// Package strict represents CLI command that displays Terragrunt's strict control settings.
// Example usage:
//
//	terragrunt info strict list        # List active strict controls
//	terragrunt info strict list --all  # List all strict controls
package strict

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName     = "strict"
	ListCommandName = "list"

	ShowAllFlagName = "all"
)

func NewListFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:    ShowAllFlagName,
			EnvVars: flags.EnvVarsWithTgPrefix(ShowAllFlagName),
			Usage:   "Show all controls, including completed ones.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Command associated with strict control settings.",
		Subcommands: cli.Commands{
			&cli.Command{
				Name:      ListCommandName,
				Flags:     NewListFlags(opts),
				Usage:     "List the strict control settings.",
				UsageText: "terragrunt info strict list [options] <name>",
				Action:    ListAction(opts),
			},
		},
		Action: cli.ShowCommandHelp,
	}
}
