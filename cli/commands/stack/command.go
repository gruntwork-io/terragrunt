package stack

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "stack"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Terragrunt stack commands.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error {
			command := ctx.Args().Get(0)
			return Run(ctx.Context, opts.OptionsFromContext(ctx), command)
		},
	}
}
