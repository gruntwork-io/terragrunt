// Package strict represents CLI command that displays Terragrunt's strict control settings.
// Example usage:
//
//	terragrunt info strict list        # List active strict controls
//	terragrunt info strict list --all  # List all strict controls
package strict

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
	"github.com/gruntwork-io/terragrunt/internal/strict/view/plaintext"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "strict"

	ListCommandName = "list"

	ShowAllFlagName = "all"
)

func NewListFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    ShowAllFlagName,
			EnvVars: tgPrefix.EnvVars(ShowAllFlagName),
			Usage:   "Show all controls, including completed ones.",
		}),
	}
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Command associated with strict control settings.",
		Subcommands: clihelper.Commands{
			&clihelper.Command{
				Name:      ListCommandName,
				Flags:     NewListFlags(opts, nil),
				Usage:     "List the strict control settings.",
				UsageText: "terragrunt info strict list [options] <name>",
				Action:    ListAction(opts),
			},
		},
		Action: clihelper.ShowCommandHelp,
	}
}

func ListAction(opts *options.TerragruntOptions) func(ctx context.Context, cliCtx *clihelper.Context) error {
	return func(_ context.Context, cliCtx *clihelper.Context) error {
		var allowedStatuses = []strict.Status{
			strict.ActiveStatus,
		}

		if val, ok := cliCtx.Flag(ShowAllFlagName).Value().Get().(bool); ok && val {
			allowedStatuses = append(allowedStatuses, strict.CompletedStatus)
		}

		controls := opts.StrictControls.FilterByStatus(allowedStatuses...)
		render := plaintext.NewRender()
		writer := view.NewWriter(cliCtx.Writer, render)

		if name := cliCtx.Args().CommandName(); name != "" {
			control := controls.Find(name)
			if control == nil {
				return strict.NewInvalidControlNameError(controls.Names())
			}

			return writer.DetailControl(control)
		}

		return writer.List(controls)
	}
}
