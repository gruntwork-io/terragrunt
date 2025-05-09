package strict

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
	"github.com/gruntwork-io/terragrunt/internal/strict/view/plaintext"
	"github.com/gruntwork-io/terragrunt/options"
)

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
