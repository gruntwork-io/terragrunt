// Package strict represents CLI command that displays Terragrunt's strict control settings.
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
)

var allowedStatuses = []strict.Status{
	strict.ActiveStatus,
	strict.CompletedStatus,
}

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Show strict control settings.",
		UsageText:            "terragrunt info strict [options] <name>",
		ErrorOnUndefinedFlag: true,
		Action:               Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
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
