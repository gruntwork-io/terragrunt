// Package find provides the ability to find Terragrunt configurations in your codebase
// via the `terragrunt find` command.
package find

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName  = "find"
	CommandAlias = "fd"

	FormatFlagName = "format"

	JSONFlagName  = "json"
	JSONFlagAlias = "j"

	DAGFlagName = "dag"

	HiddenFlagName   = "hidden"
	NoHiddenFlagName = "no-hidden"
	Dependencies     = "dependencies"
	External         = "external"
	Exclude          = "exclude"
	Include          = "include"
	Reading          = "reading"

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
					if err := opts.StrictControls.FilterByNames(controls.DeprecatedHiddenFlag).Evaluate(ctx); err != nil {
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
			Usage:   "Discover external dependencies from initial results, and add them to top-level results (implies discovery of dependencies).",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				if !value {
					return nil
				}

				opts.FilterQueries = append(opts.FilterQueries, "{./**}...")

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
	}

	return append(flags, shared.NewFilterFlags(l, opts.TerragruntOptions)...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdOpts := NewOptions(opts)

	// Base flags for find plus backend/feature flags
	flags := NewFlags(l, cmdOpts, nil)
	flags = append(flags, shared.NewBackendFlags(opts, nil)...)
	flags = append(flags, shared.NewFeatureFlags(opts, nil)...)

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
			return Run(ctx, l, cmdOpts)
		},
	}
}
