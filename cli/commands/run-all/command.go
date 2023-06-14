package runall

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "run-all"
)

var (
	TerragruntFlagNames = terraform.TerragruntFlagNames

	CommandsRunFuncs = make(map[string]func(opts *options.TerragruntOptions) error)
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.",
		UsageText:   fmt.Sprintf("terragrunt %s <terraform command> [terraform options] [global options]", CommandName),
		Flags:       flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before:      func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action:      Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		opts.RunTerragrunt = func(opts *options.TerragruntOptions) error {
			if runFunc, ok := CommandsRunFuncs[opts.TerraformCommand]; ok {
				return runFunc(opts)
			}

			return terraform.Run(opts)
		}

		return Run(opts)
	}
}
