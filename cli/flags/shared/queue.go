package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
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
func NewQueueFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIgnoreErrorsFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
				Destination: &opts.IgnoreDependencyErrors,
				Usage:       "Continue processing Units even if a dependency fails.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-errors"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIgnoreDAGOrderFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
				Destination: &opts.IgnoreDependencyOrder,
				Usage:       "Ignore DAG order for --all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-order"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueExcludeExternalFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludeExternalFlagName),
				Destination: &opts.IgnoreExternalDependencies,
				Usage:       "Ignore external dependencies for --all commands.",
				Hidden:      true,
				Action: func(ctx *cli.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueExcludeExternal).Evaluate(ctx.Context)
					}
					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIncludeExternalFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeExternalFlagName),
				Destination: &opts.IncludeExternalDependencies,
				Usage:       "Include external dependencies for --all commands without asking.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        QueueExcludesFileFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
				Destination: &opts.ExcludesFile,
				Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("excludes-file"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueExcludeDirFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludeDirFlagName),
				Destination: &opts.ExcludeDirs,
				Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("exclude-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueIncludeDirFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeDirFlagName),
				Destination: &opts.IncludeDirs,
				Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueStrictIncludeFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueStrictIncludeFlagName),
				Destination: &opts.StrictInclude,
				Usage:       "If flag is set, only modules under the directories passed in with '--queue-include-dir' will be included.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("strict-include"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueIncludeUnitsReadingFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
				Destination: &opts.UnitsReading,
				Usage:       "If flag is set, 'run --all' will only run the command against units that read the specified file via a Terragrunt HCL function or include.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("queue-include-units-reading"), terragruntPrefixControl),
		),
	}
}
