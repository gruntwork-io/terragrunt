// Package stack provides the command to stack.
package stack

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// CommandName stack command name.
	CommandName          = "stack"
	OutputFormatFlagName = "format"
	JSONFormatFlagName   = "json"
	RawFormatFlagName    = "raw"

	generateCommandName = "generate"
	runCommandName      = "run"
	outputCommandName   = "output"
	cleanCommandName    = "clean"

	rawOutputFormat  = "raw"
	jsonOutputFormat = "json"
)

// NewFlags builds the flags for stack.
func NewFlags(_ *options.TerragruntOptions) cli.Flags {
	return cli.Flags{}
}

// NewCommand builds the command for stack.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	runFlags := runall.NewFlags(opts, CommandName, nil).Filter(runall.OutDirFlagName, runall.JSONOutDirFlagName)
	runFlags = append(runFlags, run.NewFlags(opts, nil)...)

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Terragrunt stack commands.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(opts).Sort(),
		Subcommands: cli.Commands{
			&cli.Command{
				Name:  generateCommandName,
				Usage: "Generate a stack from a terragrunt.stack.hcl file",
				Action: func(ctx *cli.Context) error {
					return RunGenerate(ctx.Context, opts)

				},
			},
			&cli.Command{
				Name:  runCommandName,
				Usage: "Run a command on the stack generated from the current directory",
				Action: func(ctx *cli.Context) error {
					return Run(ctx.Context, opts)
				},
				Flags: runFlags.Sort(),
			},
			&cli.Command{
				Name:  outputCommandName,
				Usage: "Run fetch stack output",
				Action: func(ctx *cli.Context) error {
					index := ""
					if val := ctx.Args().Get(0); val != "" {
						index = val
					}
					return RunOutput(ctx.Context, opts, index)
				},
				Flags: outputFlags(opts, nil),
			},
			&cli.Command{
				Name:  cleanCommandName,
				Usage: "Clean the stack generated from the current directory",
				Action: func(ctx *cli.Context) error {
					return RunClean(ctx.Context, opts)
				},
			},
		},
		Action: cli.ShowCommandHelp,
	}
}

func outputFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutputFormatFlagName,
			EnvVars:     tgPrefix.EnvVars(OutputFormatFlagName),
			Destination: &opts.StackOutputFormat,
			Usage:       "Stack output format. Valid values are: json, raw",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:  RawFormatFlagName,
			Usage: "Stack output in raw format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = rawOutputFormat
				return nil
			},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:  JSONFormatFlagName,
			Usage: "Stack output in json format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = jsonOutputFormat
				return nil
			},
		}),
	}
}
