package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// QueueIgnoreErrorsFlagName is the flag name for `--queue-ignore-errors`, which runs a unit even when a
	// dependency fails.
	QueueIgnoreErrorsFlagName = "queue-ignore-errors"
	// QueueIgnoreDAGOrderFlagName is the flag name for `--queue-ignore-dag-order`, which runs units without
	// waiting on their dependencies.
	QueueIgnoreDAGOrderFlagName = "queue-ignore-dag-order"
	// QueueExcludeExternalFlagName is the flag name for the hidden, deprecated `--queue-exclude-external`.
	// External dependencies are excluded by default, so setting it only evaluates its strict control.
	QueueExcludeExternalFlagName = "queue-exclude-external"
	// QueueExcludeDirFlagName is the flag name for the hidden `--queue-exclude-dir`, a glob of directories
	// to drop from the queue.
	QueueExcludeDirFlagName = "queue-exclude-dir"
	// QueueExcludesFileFlagName is the flag name for the hidden `--queue-excludes-file`, a file listing
	// directories to drop from the queue.
	QueueExcludesFileFlagName = "queue-excludes-file"
	// QueueIncludeDirFlagName is the flag name for the hidden `--queue-include-dir`, a glob of directories
	// to add to the queue.
	QueueIncludeDirFlagName = "queue-include-dir"
	// QueueIncludeExternalFlagName is the flag name for the hidden `--queue-include-external`, which adds
	// dependencies outside the working directory to the queue.
	QueueIncludeExternalFlagName = "queue-include-external"
	// QueueStrictIncludeFlagName is the flag name for the hidden, deprecated `--queue-strict-include`. Its
	// behavior is the default, so setting it only evaluates its strict control.
	QueueStrictIncludeFlagName = "queue-strict-include"
	// QueueIncludeUnitsReadingFlagName is the flag name for the hidden `--queue-include-units-reading`,
	// which limits the queue to units that read the named file.
	QueueIncludeUnitsReadingFlagName = "queue-include-units-reading"
	// UnitsThatIncludeFlagName is the flag name for the hidden, deprecated `--units-that-include`, which
	// limits the queue to units that read the named file. Setting it evaluates its strict control.
	UnitsThatIncludeFlagName = "units-that-include"
)

// NewSelectionFlags creates every flag that adds to opts.Filters, which select the stacks and units a
// command acts on: the filter flags, the queue selection flags, and --units-that-include.
func NewSelectionFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	selectionFlags := clihelper.Flags{NewUnitsThatIncludeFlag(opts)}

	selectionFlags = selectionFlags.Add(NewQueueSelectionFlags(opts, nil)...)
	selectionFlags = selectionFlags.Add(NewFilterFlags(l, opts, v)...)

	return selectionFlags
}

// NewUnitsThatIncludeFlag creates the hidden, deprecated flag that limits the queue to units that read the
// named file.
func NewUnitsThatIncludeFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(
		&clihelper.SliceFlag[string]{
			Name:    UnitsThatIncludeFlagName,
			EnvVars: tgPrefix.EnvVars(UnitsThatIncludeFlagName),
			Usage:   "If flag is set, 'run --all' will only run the command against Terragrunt modules that include the specified file.",
			Hidden:  true,
			Action: func(ctx context.Context, _ *clihelper.Context, value []string) error {
				if len(value) == 0 {
					return nil
				}

				if err := opts.StrictControls.FilterByNames(controls.UnitsThatInclude).
					Evaluate(ctx); err != nil {
					return err
				}

				for _, v := range value {
					attrExpr, err := filter.NewAttributeExpression(filter.AttributeReading, v)
					if err != nil {
						return err
					}

					opts.Filters = append(
						opts.Filters,
						filter.NewFilter(attrExpr, attrExpr.String()),
					)
				}

				return nil
			},
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("modules-that-include"),
			opts.StrictControls,
		),
	)
}

// NewQueueRunFlags creates the flags that control how the queue runs its units: whether a unit waits on
// its dependencies, and whether it runs after one of them fails.
func NewQueueRunFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        QueueIgnoreErrorsFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
				Destination: &opts.IgnoreDependencyErrors,
				Usage:       "Continue processing Units even if a dependency fails.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("ignore-dependency-errors"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        QueueIgnoreDAGOrderFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
				Destination: &opts.IgnoreDependencyOrder,
				Usage:       "Ignore DAG order for --all commands.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("ignore-dependency-order"),
				opts.StrictControls,
			),
		),
	}
}

// NewQueueSelectionFlags creates the hidden flags that select the units in the queue. Each one that still
// selects anything does it by adding a filter.
func NewQueueSelectionFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    QueueExcludeExternalFlagName,
				EnvVars: tgPrefix.EnvVars(QueueExcludeExternalFlagName),
				Usage:   "Ignore external dependencies for --all commands.",
				Hidden:  true,
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueExcludeExternal).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("ignore-external-dependencies"),
				opts.StrictControls,
			),
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

					pathExpr, err := filter.NewPathFilter("./**")
					if err != nil {
						return err
					}

					graphExpr := filter.NewGraphExpression(pathExpr).WithDependencies()
					opts.Filters = append(
						opts.Filters,
						filter.NewFilter(graphExpr, graphExpr.String()),
					)

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("include-external-dependencies"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        QueueExcludesFileFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
				Destination: &opts.ExcludesFile,
				Hidden:      true,
				Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("excludes-file"),
				opts.StrictControls,
			),
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
						pathExpr, err := filter.NewPathFilter(v)
						if err != nil {
							return err
						}

						prefixExpr := filter.NewPrefixExpression("!", pathExpr)
						opts.Filters = append(
							opts.Filters,
							filter.NewFilter(prefixExpr, prefixExpr.String()),
						)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("exclude-dir"),
				opts.StrictControls,
			),
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
						pathExpr, err := filter.NewPathFilter(v)
						if err != nil {
							return err
						}

						opts.Filters = append(
							opts.Filters,
							filter.NewFilter(pathExpr, pathExpr.String()),
						)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("include-dir"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    QueueStrictIncludeFlagName,
				EnvVars: tgPrefix.EnvVars(QueueStrictIncludeFlagName),
				Usage:   "If flag is set, only modules under the directories passed in with '--queue-include-dir' will be included.",
				Hidden:  true,
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueStrictInclude).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("strict-include"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    QueueIncludeUnitsReadingFlagName,
				EnvVars: tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
				Usage:   "If flag is set, 'run --all' will only run the command against units that read the specified file via a Terragrunt HCL function or include.",
				Hidden:  true,
				Action: func(_ context.Context, _ *clihelper.Context, value []string) error {
					if len(value) == 0 {
						return nil
					}

					for _, v := range value {
						attrExpr, err := filter.NewAttributeExpression(filter.AttributeReading, v)
						if err != nil {
							return err
						}

						opts.Filters = append(
							opts.Filters,
							filter.NewFilter(attrExpr, attrExpr.String()),
						)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("queue-include-units-reading"),
				opts.StrictControls,
			),
		),
	}
}
