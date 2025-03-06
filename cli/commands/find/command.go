// Package find provides the ability to find Terragrunt configurations in your codebase
// via the `terragrunt find` command.
package find

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "find"

	FormatFlagName = "format"
	SortFlagName   = "sort"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for the find results. Valid values: text, json",
			DefaultText: "text",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SortFlagName,
			EnvVars:     tgPrefix.EnvVars(SortFlagName),
			Destination: &opts.Sort,
			Usage:       "Sort order for the find results. Valid values: alpha", // TODO: add dag
			DefaultText: "alpha",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Find relevant Terragrunt configurations.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(cmdOpts, nil),
		Before: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			if !opts.Experiments.Evaluate(experiment.Stacks) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.Stacks), cli.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx *cli.Context) error {
			return Run(ctx, cmdOpts)
		},
	}
}
