// Package list provides the ability to list Terragrunt configurations in your codebase
// via the `terragrunt list` command.
package list

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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
	NoHiddenFlagName     = "no-hidden"
	DependenciesFlagName = "dependencies"
	ExternalFlagName     = "external"

	DAGFlagName = "dag"

	QueueConstructAsFlagName  = "queue-construct-as"
	QueueConstructAsFlagAlias = "as"
)

func NewFlags(l log.Logger, opts *Options, v *venv.Venv) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	filterFlags := shared.NewFilterFlags(l, opts.TerragruntOptions, v)

	const numLocalFlags = 9

	result := make(clihelper.Flags, 0, numLocalFlags+len(filterFlags))
	result = append(result,
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for list results. Valid values: text, tree, long, dot.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoHiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHiddenFlagName),
			Destination: &opts.NoHidden,
			Usage:       "Exclude hidden directories from list results.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    HiddenFlagName,
			EnvVars: tgPrefix.EnvVars(HiddenFlagName),
			Usage:   "Include hidden directories in list results.",
			Hidden:  true,
			Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
				if value {
					if err := opts.StrictControls.FilterByNames(controls.DeprecatedHiddenFlag).
						Evaluate(ctx); err != nil {
						return err
					}
				}

				return nil
			},
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
			Usage: "Discover external dependencies from initial results," +
				" and add them to top-level results (implies discovery of dependencies).",
			Hidden: true,
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				if !value {
					return nil
				}

				pathExpr, err := filter.NewPathFilter("./**")
				if err != nil {
					return err
				}

				graphExpr := filter.NewGraphExpression(pathExpr).WithDependencies()
				opts.Filters = append(opts.Filters, filter.NewFilter(graphExpr, graphExpr.String()))

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
	)

	flags.DeprecateEnvVarPrefix(result, flags.Prefix{flags.TgPrefix, CommandName}, opts.StrictControls)

	return append(result, filterFlags...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmdOpts := NewOptions(opts)

	sharedFlags := shared.NewBackendFlags(opts, nil)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, nil)...)
	sharedFlags = append(sharedFlags, shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil))
	flags.DeprecateEnvVarPrefix(sharedFlags, flags.Prefix{flags.TgPrefix, CommandName}, opts.StrictControls)

	// Base flags for list plus backend/feature flags
	flags := append(NewFlags(l, cmdOpts, v), sharedFlags...)

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
			return Run(ctx, l, v, cmdOpts)
		},
	}
}
