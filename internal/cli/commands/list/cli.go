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
	// CommandName is the name of the `terragrunt list` command.
	CommandName = "list"
	// CommandAlias is the short alias of `terragrunt list`.
	CommandAlias = "ls"

	// FormatFlagName is the name of the flag that selects the output format:
	// [FormatText], [FormatTree], [FormatLong], or [FormatDot].
	FormatFlagName = "format"

	// TreeFlagName is the name of the flag that is shorthand for --format=tree.
	TreeFlagName = "tree"
	// TreeFlagAlias is the short alias of the --tree flag.
	TreeFlagAlias = "T"

	// LongFlagName is the name of the flag that is shorthand for --format=long.
	LongFlagName = "long"
	// LongFlagAlias is the short alias of the --long flag.
	LongFlagAlias = "l"

	// HiddenFlagName is the name of the hidden, deprecated flag that includes
	// hidden directories. Setting it evaluates the deprecated hidden flag
	// strict control.
	HiddenFlagName = "hidden"
	// NoHiddenFlagName is the name of the flag that excludes hidden directories
	// from the results.
	NoHiddenFlagName = "no-hidden"
	// DependenciesFlagName is the name of the flag that adds each component's
	// dependencies to the output.
	DependenciesFlagName = "dependencies"
	// ExternalFlagName is the name of the hidden flag that adds the
	// dependencies of every discovered component, including those outside the
	// working directory, to the results.
	ExternalFlagName = "external"

	// DAGFlagName is the name of the flag that sorts and groups output in DAG
	// order.
	DAGFlagName = "dag"

	// QueueConstructAsFlagName is the name of the flag that orders the results
	// as the run queue for the given command would. It implies DAG mode.
	QueueConstructAsFlagName = "queue-construct-as"
	// QueueConstructAsFlagAlias is the short alias of the --queue-construct-as
	// flag.
	QueueConstructAsFlagAlias = "as"
)

// NewFlags returns the flags of `terragrunt list`, bound to opts.
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

// NewCommand returns the `terragrunt list` command.
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
