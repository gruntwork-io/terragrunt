// Package stack provides the command to stack.
package stack

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName stack command name.
	CommandName = "stack"
	generate    = "generate"
	run         = "run"
	output      = "output"
)

// NewFlags builds the flags for stack.
func NewFlags(_ *options.TerragruntOptions) cli.Flags {
	return cli.Flags{}
}

// NewCommand builds the command for stack.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Terragrunt stack commands.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
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
			&cli.Command{
				Name:  output,
				Usage: "Run fetch stack output",
				Action: func(ctx *cli.Context) error {
					return RunOutput(ctx.Context, opts.OptionsFromContext(ctx))
				},
			},
		},
		Action: func(ctx *cli.Context) error {
			return cli.ShowCommandHelp(ctx, generate)
		},
	}
}
