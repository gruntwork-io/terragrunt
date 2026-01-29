package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	QueueIgnoreErrorsFlagName        = "queue-ignore-errors"
	QueueIgnoreDAGOrderFlagName      = "queue-ignore-dag-order"
	QueueExcludeExternalFlagName     = "queue-exclude-external"
	QueueExcludeDirFlagName          = "queue-exclude-dir"
	QueueExcludesFileFlagName        = "queue-excludes-file"
	QueueIncludeDirFlagName          = "queue-include-dir"
	QueueIncludeExternalFlagName     = "queue-include-external"
	QueueStrictIncludeFlagName       = "queue-strict-include"
	QueueIncludeUnitsReadingFlagName = "queue-include-units-reading"
)

// NewQueueFlags creates the flags used for queue control
func NewQueueFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        QueueIgnoreErrorsFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
				Destination: &opts.IgnoreDependencyErrors,
				Usage:       "Continue processing Units even if a dependency fails.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-errors"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        QueueIgnoreDAGOrderFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
				Destination: &opts.IgnoreDependencyOrder,
				Usage:       "Ignore DAG order for --all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-order"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    QueueExcludeExternalFlagName,
				EnvVars: tgPrefix.EnvVars(QueueExcludeExternalFlagName),
				Usage:   "Ignore external dependencies for --all commands.",
				Hidden:  true,
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueExcludeExternal).Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    QueueIncludeExternalFlagName,
				EnvVars: tgPrefix.EnvVars(QueueIncludeExternalFlagName),
				Usage:   "Include external dependencies for --all commands.",
				Hidden:  true,
				Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
					if !value {
						return nil
					}

					opts.FilterQueries = append(opts.FilterQueries, "{./**}...")

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        QueueExcludesFileFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
				Destination: &opts.ExcludesFile,
				Hidden:      true,
				Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("excludes-file"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    QueueExcludeDirFlagName,
				EnvVars: tgPrefix.EnvVars(QueueExcludeDirFlagName),
				Hidden:  true,
				Usage:   "Unix-style glob of directories to exclude from the queue of Units to run.",
				Action: func(_ context.Context, _ *clihelper.Context, value []string) error {
					if len(value) == 0 {
						return nil
					}

					for _, v := range value {
						// We explicitly wrap the value in curly braces to ensure that it is treated
						// as a path expression, and not a name filter.
						opts.FilterQueries = append(opts.FilterQueries, "!{"+v+"}")
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("exclude-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    QueueIncludeDirFlagName,
				EnvVars: tgPrefix.EnvVars(QueueIncludeDirFlagName),
				Hidden:  true,
				Usage:   "Unix-style glob of directories to include from the queue of Units to run.",
				Action: func(_ context.Context, _ *clihelper.Context, value []string) error {
					if len(value) == 0 {
						return nil
					}

					for _, v := range value {
						// We explicitly wrap the value in curly braces to ensure that it is treated
						// as a path expression, and not a name filter.
						opts.FilterQueries = append(opts.FilterQueries, "{"+v+"}")
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    QueueStrictIncludeFlagName,
				EnvVars: tgPrefix.EnvVars(QueueStrictIncludeFlagName),
				Usage: "If flag is set, only modules under the directories passed in with " +
					"'--queue-include-dir' will be included.",
				Hidden: true,
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueStrictInclude).Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("strict-include"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    QueueIncludeUnitsReadingFlagName,
				EnvVars: tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
				Usage: "If flag is set, 'run --all' will only run the command against units " +
					"that read the specified file via a Terragrunt HCL function or include.",
				Hidden: true,
				Action: func(_ context.Context, _ *clihelper.Context, value []string) error {
					if len(value) == 0 {
						return nil
					}

					for _, v := range value {
						opts.FilterQueries = append(opts.FilterQueries, "reading="+v)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("queue-include-units-reading"), terragruntPrefixControl),
		),
	}
}
