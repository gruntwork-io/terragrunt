// Package list provides the ability to list Terragrunt configurations in your codebase
// via the `terragrunt list` command.
package list

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

func NewFlags(l log.Logger, opts *Options, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for list results. Valid values: text, tree, long, dot.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in list results.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DependenciesFlagName,
			EnvVars:     tgPrefix.EnvVars(DependenciesFlagName),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in list results (only when using --long).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    ExternalFlagName,
			EnvVars: tgPrefix.EnvVars(ExternalFlagName),
			Usage:   "Discover external dependencies from initial results, and add them to top-level results (implies discovery of dependencies).",
			Hidden:  true,
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				if !value {
					return nil
				}

				opts.FilterQueries = append(opts.FilterQueries, "{./**}...")
				return nil
			},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        TreeFlagName,
			EnvVars:     tgPrefix.EnvVars(TreeFlagName),
			Destination: &opts.Tree,
			Usage:       "Output in tree format (equivalent to --format=tree).",
			Aliases:     []string{TreeFlagAlias},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        LongFlagName,
			EnvVars:     tgPrefix.EnvVars(LongFlagName),
			Destination: &opts.Long,
			Usage:       "Output in long format (equivalent to --format=long).",
			Aliases:     []string{LongFlagAlias},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     tgPrefix.EnvVars(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        QueueConstructAsFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueConstructAsFlagName),
			Destination: &opts.QueueConstructAs,
			Usage:       "Construct the queue as if a specific command was run.",
			Aliases:     []string{QueueConstructAsFlagAlias},
		}),
	}

	return append(flags, shared.NewFilterFlags(l, opts.TerragruntOptions)...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdOpts := NewOptions(opts)
	prefix := flags.Prefix{CommandName}

	// Base flags for list plus backend/feature flags
	flags := NewFlags(l, cmdOpts, prefix)
	flags = append(flags, shared.NewBackendFlags(opts, prefix)...)
	flags = append(flags, shared.NewFeatureFlags(opts, prefix)...)

	return &clihelper.Command{
		Name:    CommandName,
		Aliases: []string{CommandAlias},
		Usage:   "List relevant Terragrunt configurations.",
		Flags:   flags,
		Before: func(_ context.Context, _ *clihelper.Context) error {
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
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, cmdOpts)
		},
	}
}
