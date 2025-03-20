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
	CommandName  = "find"
	CommandAlias = "fd"

	FormatFlagName = "format"
	JSONFlagName   = "json"
	SortFlagName   = "sort"
	HiddenFlagName = "hidden"
	Dependencies   = "dependencies"
	External       = "external"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for the find results. Valid values: text, json.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Destination: &opts.JSON,
			Usage:       "Output in JSON format (equivalent to --format=json).",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SortFlagName,
			EnvVars:     tgPrefix.EnvVars(SortFlagName),
			Destination: &opts.Sort,
			Usage:       "Sort order for the find results. Valid values: alpha, dag.",
			DefaultText: SortAlpha,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in find results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Dependencies,
			EnvVars:     tgPrefix.EnvVars(Dependencies),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in the results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        External,
			EnvVars:     tgPrefix.EnvVars(External),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	return &cli.Command{
		Name:                 CommandName,
		Aliases:              []string{CommandAlias},
		Usage:                "Find relevant Terragrunt configurations.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(cmdOpts, nil),
		Before: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			if cmdOpts.JSON {
				cmdOpts.Format = FormatJSON
			}

			if err := cmdOpts.Validate(); err != nil {
				return cli.NewExitError(err, cli.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx *cli.Context) error {
			return Run(ctx, cmdOpts)
		},
	}
}
