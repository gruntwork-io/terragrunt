// Package stack provides the command to stack.
package stack

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// CommandName stack command name.
	CommandName = "stack"
	generate    = "generate"
	run         = "run"
)

// NewFlags builds the flags for stack.
func NewFlags(_ *options.TerragruntOptions, _ flags.Prefix) cli.Flags {
	return cli.Flags{}
}

// NewCommand builds the command for stack.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Terragrunt stack commands.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(opts, nil).Sort(),
		Subcommands: cli.Commands{
			&cli.Command{
				Name:  generate,
				Usage: "Generate a stack from a terragrunt.stack.hcl file",
				Action: func(ctx *cli.Context) error {
					return RunGenerate(ctx.Context, opts.OptionsFromContext(ctx))

				},
			},
			&cli.Command{
				Name:  run,
				Usage: "Run a command on the stack generated from the current directory",
				Action: func(ctx *cli.Context) error {
					return Run(ctx.Context, opts.OptionsFromContext(ctx))
				},
			},
		},
		Action: cli.ShowCommandHelp,
	}
}
