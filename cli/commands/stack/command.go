// Package stack provides the command to stack.
package stack

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// CommandName stack command name.
	CommandName          = "stack"
	OutputFormatFlagName = "format"
	OutputFormatEnvName  = "TERRAGRUNT_STACK_OUTPUT_FORMAT"
	JSONFormatFlagName   = "json"
	RawFormatFlagName    = "raw"

	generate = "generate"
	run      = "run"
	output   = "output"

	rawOutputFormat  = "raw"
	jsonOutputFormat = "json"
)

// NewFlags builds the flags for stack.
func NewFlags(opts *options.TerragruntOptions, _ flags.Prefix) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:   OutputFormatFlagName,
			EnvVar: OutputFormatEnvName,

			Destination: &opts.StackOutputFormat,
			Usage:       "Stack output format. Valid values are: json, raw",
		},
		&cli.BoolFlag{
			Name:  RawFormatFlagName,
			Usage: "Stack output in raw format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = rawOutputFormat
				return nil
			},
		},
		&cli.BoolFlag{
			Name:  JSONFormatFlagName,
			Usage: "Stack output in json format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = jsonOutputFormat
				return nil
			},
		},
	}
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
			&cli.Command{
				Name:  output,
				Usage: "Run fetch stack output",
				Action: func(ctx *cli.Context) error {
					index := ""
					if val := ctx.Args().Get(0); val != "" {
						index = val
					}
					return RunOutput(ctx.Context, opts.OptionsFromContext(ctx), index)
				},
			},
		},
		Action: cli.ShowCommandHelp,
	}
}
