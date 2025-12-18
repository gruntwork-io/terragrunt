package shared

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	FilterFlagName             = "filter"
	FilterAffectedFlagName     = "filter-affected"
	FilterAllowDestroyFlagName = "filter-allow-destroy"
	FilterFileFlagName         = "filters-file"
	NoFilterFileFlagName       = "no-filters-file"
)

// NewFilterFlags creates flags for specifying filter queries.
func NewFilterFlags(l log.Logger, opts *options.TerragruntOptions) cli.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return cli.Flags{
		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        FilterFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterFlagName),
				Destination: &opts.FilterQueries,
				Usage:       "Filter components using filter syntax. Can be specified multiple times for union (OR) semantics. Requires the 'filter' experiment.",
				Action: func(_ *cli.Context, val []string) error {
					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filter flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
		flags.NewFlag(
			&cli.BoolFlag{
				Name:    FilterAffectedFlagName,
				EnvVars: tgPrefix.EnvVars(FilterAffectedFlagName),
				Usage:   "Filter components affected by changes between main and HEAD. Equivalent to --filter=[main...HEAD]. Requires the 'filter-flag' experiment.",
				Action: func(ctx *cli.Context, val bool) error {
					if !val {
						return nil
					}

					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filter-affected flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}

					// Get working directory
					workDir := opts.WorkingDir
					if workDir == "" {
						workDir = opts.RootWorkingDir
					}
					if workDir == "" {
						// Fallback to current directory if neither is set
						workDir = "."
					}

					// Check for uncommitted changes
					gitRunner, err := git.NewGitRunner()
					if err != nil {
						return cli.NewExitError(err, cli.ExitCodeGeneralError)
					}

					gitRunner = gitRunner.WithWorkDir(workDir)

					if gitRunner.HasUncommittedChanges(ctx.Context) {
						l.Warnf("Warning: You have uncommitted changes. The --filter-affected flag may not include all your local modifications.")
					}

					defaultBranch := gitRunner.GetDefaultBranch(ctx.Context, l)

					opts.FilterQueries = append(opts.FilterQueries, fmt.Sprintf("[%s...HEAD]", defaultBranch))

					return nil
				},
			},
		),
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        FilterAllowDestroyFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterAllowDestroyFlagName),
				Destination: &opts.FilterAllowDestroy,
				Usage:       "Allow destroy runs when using Git-based filters. Requires the 'filter-flag' experiment.",
				Action: func(_ *cli.Context, val bool) error {
					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filter-allow-destroy flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        FilterFileFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterFileFlagName),
				Destination: &opts.FiltersFile,
				Usage:       "Path to a file containing filter queries, one per line. Default is .terragrunt-filters. Requires the 'filter-flag' experiment.",
				Action: func(_ *cli.Context, val string) error {
					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filters-file flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        NoFilterFileFlagName,
				EnvVars:     tgPrefix.EnvVars(NoFilterFileFlagName),
				Destination: &opts.NoFiltersFile,
				Usage:       "Disable automatic reading of .terragrunt-filters file. Requires the 'filter-flag' experiment.",
				Action: func(_ *cli.Context, val bool) error {
					if !val {
						return nil
					}

					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --no-filters-file flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
	}
}
