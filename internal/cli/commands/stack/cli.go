// Package stack provides the command to stack.
package stack

import (
	"context"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// CommandName stack command name.
	CommandName          = "stack"
	OutputFormatFlagName = "format"
	JSONFormatFlagName   = "json"
	RawFormatFlagName    = "raw"
	NoStackValidate      = "no-stack-validate"

	generateCommandName = "generate"
	runCommandName      = "run"
	outputCommandName   = "output"
	cleanCommandName    = "clean"

	rawOutputFormat  = "raw"
	jsonOutputFormat = "json"
)

// NewCommand builds the command for stack.
func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Terragrunt stack commands.",
		Subcommands: clihelper.Commands{
			&clihelper.Command{
				Name:  generateCommandName,
				Usage: "Generate a stack from a terragrunt.stack.hcl file",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return RunGenerate(ctx, l, opts.OptionsFromContext(ctx))
				},
				Flags: defaultFlags(l, opts, nil),
			},
			&clihelper.Command{
				Name:  runCommandName,
				Usage: "Run a command on the stack generated from the current directory",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return Run(ctx, l, opts.OptionsFromContext(ctx))
				},
				Flags: defaultFlags(l, opts, nil),
			},
			&clihelper.Command{
				Name:  outputCommandName,
				Usage: "Run fetch stack output",
				Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
					index := ""
					if val := cliCtx.Args().Get(0); val != "" {
						index = val
					}
					return RunOutput(ctx, l, opts.OptionsFromContext(ctx), index)
				},
				Flags: outputFlags(l, opts, nil),
			},
			&clihelper.Command{
				Name:  cleanCommandName,
				Usage: "Clean the stack generated from the current directory",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return RunClean(ctx, l, opts.OptionsFromContext(ctx))
				},
			},
		},
		Action: clihelper.ShowCommandHelp,
	}
}

func defaultFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoStackValidate,
			EnvVars:     tgPrefix.EnvVars(NoStackValidate),
			Destination: &opts.NoStackValidate,
			Hidden:      true,
			Usage:       "Disable automatic stack validation after generation.",
		}),
	}

	return append(runcmd.NewFlags(l, opts, nil), flags...)
}

func outputFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutputFormatFlagName,
			EnvVars:     tgPrefix.EnvVars(OutputFormatFlagName),
			Destination: &opts.StackOutputFormat,
			Usage:       "Stack output format. Valid values are: json, raw",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:  RawFormatFlagName,
			Usage: "Stack output in raw format",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				opts.StackOutputFormat = rawOutputFormat
				return nil
			},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:  JSONFormatFlagName,
			Usage: "Stack output in json format",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				opts.StackOutputFormat = jsonOutputFormat
				return nil
			},
		}),
	}

	return append(defaultFlags(l, opts, prefix), flags...)
}
