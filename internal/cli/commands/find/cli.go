// Package find provides the ability to find Terragrunt configurations in your codebase
// via the `terragrunt find` command.
package find

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt find` command.
	CommandName = "find"
	// CommandAlias is the short alias of `terragrunt find`.
	CommandAlias = "fd"

	// FormatFlagName is the name of the flag that selects the output format:
	// [FormatText] or [FormatJSON].
	FormatFlagName = "format"

	// JSONFlagName is the name of the flag that is shorthand for --format=json.
	JSONFlagName = "json"
	// JSONFlagAlias is the short alias of the --json flag.
	JSONFlagAlias = "j"

	// DAGFlagName is the name of the flag that sorts and groups output in DAG
	// order.
	DAGFlagName = "dag"

	// HiddenFlagName is the name of the hidden, deprecated flag that includes
	// hidden directories. Setting it evaluates the deprecated hidden flag
	// strict control.
	HiddenFlagName = "hidden"
	// NoHiddenFlagName is the name of the flag that excludes hidden directories
	// from the results.
	NoHiddenFlagName = "no-hidden"
	// Dependencies is the name of the flag that adds each component's
	// dependencies to JSON output.
	Dependencies = "dependencies"
	// External is the name of the hidden flag that adds the dependencies of
	// every discovered component, including those outside the working
	// directory, to the results.
	External = "external"
	// Exclude is the name of the flag that adds each unit's exclude block to
	// JSON output.
	Exclude = "exclude"
	// Include is the name of the flag that adds each unit's include paths to
	// JSON output.
	Include = "include"
	// Reading is the name of the flag that adds the files each component reads
	// to JSON output.
	Reading = "reading"

	// QueueConstructAsFlagName is the name of the flag that orders the results
	// as the run queue for the given command would. It implies DAG mode.
	QueueConstructAsFlagName = "queue-construct-as"
	// QueueConstructAsFlagAlias is the short alias of the --queue-construct-as
	// flag.
	QueueConstructAsFlagAlias = "as"
)

// NewFlags returns the flags of `terragrunt find`, bound to opts.
func NewFlags(l log.Logger, opts *Options, v *venv.Venv, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	filterFlags := shared.NewFilterFlags(l, opts.TerragruntOptions, v)

	const numLocalFlags = 11

	result := make(clihelper.Flags, 0, numLocalFlags+len(filterFlags))
	result = append(result,
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for find results. Valid values: text, json.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Aliases:     []string{JSONFlagAlias},
			Destination: &opts.JSON,
			Usage:       "Output in JSON format (equivalent to --format=json).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     tgPrefix.EnvVars(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoHiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHiddenFlagName),
			Destination: &opts.NoHidden,
			Usage:       "Exclude hidden directories from find results.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    HiddenFlagName,
			EnvVars: tgPrefix.EnvVars(HiddenFlagName),
			Usage:   "Include hidden directories in find results.",
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
			Name:        Dependencies,
			EnvVars:     tgPrefix.EnvVars(Dependencies),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in the results (only when using --format=json).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        Exclude,
			EnvVars:     tgPrefix.EnvVars(Exclude),
			Destination: &opts.Exclude,
			Usage:       "Display exclude configurations in the results (only when using --format=json).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        Include,
			EnvVars:     tgPrefix.EnvVars(Include),
			Destination: &opts.Include,
			Usage:       "Display include configurations in the results (only when using --format=json).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        Reading,
			EnvVars:     tgPrefix.EnvVars(Reading),
			Destination: &opts.Reading,
			Usage:       "Include the list of files that are read by components in the results (only when using --format=json).",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    External,
			EnvVars: tgPrefix.EnvVars(External),
			Hidden:  true,
			Usage: "Discover external dependencies from initial results," +
				" and add them to top-level results (implies discovery of dependencies).",
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
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        QueueConstructAsFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueConstructAsFlagName),
			Destination: &opts.QueueConstructAs,
			Usage:       "Construct the queue as if a specific command was run.",
			Aliases:     []string{QueueConstructAsFlagAlias},
		}),
	)

	return append(result, filterFlags...)
}

// NewCommand returns the `terragrunt find` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	cmdOpts := NewOptions(opts)

	// Base flags for find plus backend/feature flags
	flags := NewFlags(l, cmdOpts, v, nil)
	flags = append(flags, shared.NewBackendFlags(opts, nil)...)
	flags = append(flags, shared.NewFeatureFlags(opts, nil)...)
	flags = append(flags, shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil))

	return &clihelper.Command{
		Name:    CommandName,
		Aliases: []string{CommandAlias},
		Usage:   "Find relevant Terragrunt configurations.",
		Flags:   flags,
		Before: func(_ context.Context, _ *clihelper.Context) error {
			if cmdOpts.JSON {
				cmdOpts.Format = FormatJSON
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
			return Run(ctx, l, v, s, cmdOpts)
		},
	}
}
