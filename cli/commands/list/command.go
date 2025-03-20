// Package list provides the ability to list Terragrunt configurations in your codebase
// via the `terragrunt list` command.
package list

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName  = "list"
	CommandAlias = "ls"

	FormatFlagName = "format"

	TreeFlagName  = "tree"
	TreeFlagAlias = "T"

	LongFlagName  = "long"
	LongFlagAlias = "l"

	HiddenFlagName       = "hidden"
	DependenciesFlagName = "dependencies"
	ExternalFlagName     = "external"

	DAGFlagName = "dag"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for list results. Valid values: text, tree, long.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in list results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DependenciesFlagName,
			EnvVars:     tgPrefix.EnvVars(DependenciesFlagName),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in list results (only when using --long).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(ExternalFlagName),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results, and add them to top-level results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        TreeFlagName,
			EnvVars:     tgPrefix.EnvVars(TreeFlagName),
			Destination: &opts.Tree,
			Usage:       "Output in tree format (equivalent to --format=tree).",
			Aliases:     []string{TreeFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        LongFlagName,
			EnvVars:     tgPrefix.EnvVars(LongFlagName),
			Destination: &opts.Long,
			Usage:       "Output in long format (equivalent to --format=long).",
			Aliases:     []string{LongFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     tgPrefix.EnvVars(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	return &cli.Command{
		Name:                 CommandName,
		Aliases:              []string{CommandAlias},
		Usage:                "List relevant Terragrunt configurations.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(cmdOpts, nil),
		Before: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			if cmdOpts.Tree {
				cmdOpts.Format = FormatTree
			}

			if cmdOpts.Long {
				cmdOpts.Format = FormatLong
			}

			if cmdOpts.DAG {
				cmdOpts.Mode = ModeDAG
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
