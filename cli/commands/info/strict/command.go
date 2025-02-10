// Package strict represents CLI command that displays Terragrunt's strict control settings.
// Example usage:
//
//	terragrunt info strict list        # List active strict controls
//	terragrunt info strict list --all  # List all strict controls
package strict

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
	"github.com/gruntwork-io/terragrunt/internal/strict/view/plaintext"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "strict"

	ListCommandName = "list"

	ShowAllFlagName = "all"
)

func NewListFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:    ShowAllFlagName,
			EnvVars: tgPrefix.EnvVars(ShowAllFlagName),
			Usage:   "Show all controls, including completed ones.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	prefix = prefix.Append(CommandName)

	return &cli.Command{
		Name:  CommandName,
		Usage: "Command associated with strict control settings.",
		Subcommands: cli.Commands{
			&cli.Command{
				Name:                 ListCommandName,
				Flags:                NewListFlags(opts, prefix),
				Usage:                "List the strict control settings.",
				UsageText:            "terragrunt info strict list [options] <name>",
				ErrorOnUndefinedFlag: true,
				Action:               ListAction(opts),
			},
		},
		ErrorOnUndefinedFlag: true,
		Action:               cli.ShowCommandHelp,
	}
}

func ListAction(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		var allowedStatuses = []strict.Status{
			strict.ActiveStatus,
		}

		if val, ok := ctx.Flag(ShowAllFlagName).Value().Get().(bool); ok && val {
			allowedStatuses = append(allowedStatuses, strict.CompletedStatus)
		}

		controls := opts.StrictControls.FilterByStatus(allowedStatuses...)
		render := plaintext.NewRender()
		writer := view.NewWriter(ctx.App.Writer, render)

		if name := ctx.Args().CommandName(); name != "" {
			control := controls.Find(name)
			if control == nil {
				return strict.NewInvalidControlNameError(controls.Names())
			}

			return writer.DetailControl(control)
		}

		return writer.List(controls)
	}
}
