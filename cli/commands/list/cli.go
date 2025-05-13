// Package list provides the ability to list Terragrunt configurations in your codebase
// via the `terragrunt list` command.
package list

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
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

	QueueConstructAsFlagName  = "queue-construct-as"
	QueueConstructAsFlagAlias = "as"
)

func NewFlags(opts *Options) cli.Flags {
	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(FormatFlagName),
			ConfigKey:   flags.ConfigKey(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for list results. Valid values: text, tree, long.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(HiddenFlagName),
			ConfigKey:   flags.ConfigKey(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in list results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DependenciesFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DependenciesFlagName),
			ConfigKey:   flags.ConfigKey(DependenciesFlagName),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in list results (only when using --long).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ExternalFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ExternalFlagName),
			ConfigKey:   flags.ConfigKey(ExternalFlagName),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results, and add them to top-level results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        TreeFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(TreeFlagName),
			ConfigKey:   flags.ConfigKey(TreeFlagName),
			Destination: &opts.Tree,
			Usage:       "Output in tree format (equivalent to --format=tree).",
			Aliases:     []string{TreeFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        LongFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(LongFlagName),
			ConfigKey:   flags.ConfigKey(LongFlagName),
			Destination: &opts.Long,
			Usage:       "Output in long format (equivalent to --format=long).",
			Aliases:     []string{LongFlagAlias},
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DAGFlagName),
			ConfigKey:   flags.ConfigKey(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueConstructAsFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueConstructAsFlagName),
			ConfigKey:   flags.ConfigKey(QueueConstructAsFlagName),
			Destination: &opts.QueueConstructAs,
			Usage:       "Construct the queue as if a specific command was run.",
			Aliases:     []string{QueueConstructAsFlagAlias},
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	return &cli.Command{
		Name:    CommandName,
		Aliases: []string{CommandAlias},
		Usage:   "List relevant Terragrunt configurations.",
		Flags:   NewFlags(cmdOpts),
		Before: func(ctx *cli.Context) error {
			if cmdOpts.Tree {
				cmdOpts.Format = FormatTree
			}

			if cmdOpts.Long {
				cmdOpts.Format = FormatLong
			}

			if cmdOpts.DAG {
				cmdOpts.Mode = ModeDAG
			}

			// Requesting a specific command to be used for queue construction
			// implies DAG mode.
			if cmdOpts.QueueConstructAs != "" {
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
