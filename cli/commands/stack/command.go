package stack

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "stack"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		commands.NewNoIncludeRootFlag(opts),
		commands.NewRootFileNameFlag(opts),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Terragrunt stack commands.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error {
			return Run(ctx.Context, opts.OptionsFromContext(ctx))
		},
	}
}
