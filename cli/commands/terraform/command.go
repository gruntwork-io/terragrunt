package terraform

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "terraform"
)

var (
	TerragruntFlagNames = append(flags.CommonFlagNames,
		flags.FlagNameTerragruntConfig,
	)
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: "*",
		Usage:    "Terragrunt forwards all other commands directly to Terraform",
		Flags:    flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before:   func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action:   Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == CommandNameDestroy {
			opts.CheckDependentModules = true
		}

		return Run(opts)
	}
}
