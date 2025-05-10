// Package stack provides the command to stack.
package stack

import (
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
	NoStackGenerate      = "no-stack-generate"
	NoStackValidate      = "no-stack-validate"

	generateCommandName = "generate"
	runCommandName      = "run"
	outputCommandName   = "output"
	cleanCommandName    = "clean"

	rawOutputFormat  = "raw"
	jsonOutputFormat = "json"
)

// NewCommand builds the command for stack.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdPrefix := flags.Name{CommandName}

	return &cli.Command{
		Name:  CommandName,
		Usage: "Terragrunt stack commands.",
		Subcommands: cli.Commands{
			&cli.Command{
				Name:  generateCommandName,
				Usage: "Generate a stack from a terragrunt.stack.hcl file",
				Action: func(ctx *cli.Context) error {
					return RunGenerate(ctx.Context, opts.OptionsFromContext(ctx))
				},
				Flags: defaultFlags(opts, cmdPrefix),
			},
			&cli.Command{
				Name:  runCommandName,
				Usage: "Run a command on the stack generated from the current directory",
				Action: func(ctx *cli.Context) error {
					return Run(ctx.Context, opts.OptionsFromContext(ctx))
				},
				Flags: defaultFlags(opts, cmdPrefix),
			},
			&cli.Command{
				Name:  outputCommandName,
				Usage: "Run fetch stack output",
				Action: func(ctx *cli.Context) error {
					index := ""
					if val := ctx.Args().Get(0); val != "" {
						index = val
					}
					return RunOutput(ctx.Context, opts.OptionsFromContext(ctx), index)
				},
				Flags: outputFlags(opts, cmdPrefix),
			},
			&cli.Command{
				Name:  cleanCommandName,
				Usage: "Clean the stack generated from the current directory",
				Action: func(ctx *cli.Context) error {
					return RunClean(ctx.Context, opts.OptionsFromContext(ctx))
				},
			},
		},
		Action: cli.ShowCommandHelp,
	}
}

func defaultFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        NoStackGenerate,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoStackGenerate),
			ConfigKey:   cmdPrefix.ConfigKey(NoStackGenerate),
			Destination: &opts.NoStackGenerate,
			Usage:       "Disable automatic stack regeneration before running the command.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        NoStackValidate,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoStackValidate),
			ConfigKey:   cmdPrefix.ConfigKey(NoStackValidate),
			Destination: &opts.NoStackValidate,
			Hidden:      true,
			Usage:       "Disable automatic stack validation after generation.",
		}),
	}

	return append(run.NewFlags(opts), flags...)
}

func outputFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	flags := cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutputFormatFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(OutputFormatFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(OutputFormatFlagName),
			Destination: &opts.StackOutputFormat,
			Usage:       "Stack output format. Valid values are: json, raw",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:      RawFormatFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(RawFormatFlagName),
			ConfigKey: cmdPrefix.ConfigKey(RawFormatFlagName),
			Usage:     "Stack output in raw format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = rawOutputFormat
				return nil
			},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:      JSONFormatFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(JSONFormatFlagName),
			ConfigKey: cmdPrefix.ConfigKey(JSONFormatFlagName),
			Usage:     "Stack output in json format",
			Action: func(ctx *cli.Context, value bool) error {
				opts.StackOutputFormat = jsonOutputFormat
				return nil
			},
		}),
	}

	return append(defaultFlags(opts, cmdPrefix), flags...)
}
