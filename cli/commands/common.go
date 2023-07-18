package commands

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

func Action(opts *options.TerragruntOptions, runFn func(opts *options.TerragruntOptions) error) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if err := ctx.App.Action(ctx); err != nil {
			return err
		}

		return runFn(opts.OptionsFromContext(ctx))
	}
}
