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

	FormatFlagName       = "format"
	JSONFlagName         = "json"
	TreeFlagName         = "tree"
	TreeFlagAlias        = "T"
	SortFlagName         = "sort"
	HiddenFlagName       = "hidden"
	DependenciesFlagName = "dependencies"
	ExternalFlagName     = "external"
	LongFlagName         = "long"
	LongFlagAlias        = "l"
	GroupByFlagName      = "group-by"
	DAGFlagName          = "dag"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for list results. Valid values: text, json.",
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
			Usage:       "Sort order for list results. Valid values: alpha, dag.",
			DefaultText: SortAlpha,
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        GroupByFlagName,
			EnvVars:     tgPrefix.EnvVars(GroupByFlagName),
			Destination: &opts.GroupBy,
			Usage:       "Group results by filesystem or DAG relationships. Valid values: fs, dag.",
			DefaultText: GroupByFS,
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
			Usage:       "Include dependencies in list results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(ExternalFlagName),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        TreeFlagName,
			EnvVars:     tgPrefix.EnvVars(TreeFlagName),
			Destination: &opts.Tree,
			Usage:       "Output in tree format.",
			Aliases:     []string{TreeFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        LongFlagName,
			EnvVars:     tgPrefix.EnvVars(LongFlagName),
			Destination: &opts.Long,
			Usage:       "Output in long format.",
			Aliases:     []string{LongFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     tgPrefix.EnvVars(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Output in DAG format.",
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

			if cmdOpts.JSON {
				cmdOpts.Format = FormatJSON
			}

			if cmdOpts.Tree {
				cmdOpts.Format = FormatTree
			}

			if cmdOpts.Long {
				cmdOpts.Format = FormatLong
			}

			if cmdOpts.DAG {
				cmdOpts.Sort = SortDAG
				cmdOpts.GroupBy = GroupByDAG
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
