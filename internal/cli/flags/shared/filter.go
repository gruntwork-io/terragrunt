package shared

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	FilterFlagName             = "filter"
	FilterAffectedFlagName     = "filter-affected"
	FilterAllowDestroyFlagName = "filter-allow-destroy"
	FilterFileFlagName         = "filters-file"
	NoFilterFileFlagName       = "no-filters-file"
)

// NewFilterFlags creates flags for specifying filter queries.
func NewFilterFlags(l log.Logger, opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:        FilterFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterFlagName),
				Destination: &opts.FilterQueries,
				Usage:       "Filter components using filter syntax. Can be specified multiple times for union (OR) semantics.",
				Action: func(_ context.Context, _ *clihelper.Context, val []string) error {
					if len(val) == 0 {
						return nil
					}

					opts.RunAll = true

					return nil
				},
			},
		),
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    FilterAffectedFlagName,
				EnvVars: tgPrefix.EnvVars(FilterAffectedFlagName),
				Usage:   "Filter components affected by changes between main and HEAD. Equivalent to --filter=[main...HEAD].",
				Action: func(ctx context.Context, _ *clihelper.Context, val bool) error {
					if !val {
						return nil
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
						return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
					}

					gitRunner = gitRunner.WithWorkDir(workDir)

					if gitRunner.HasUncommittedChanges(ctx) {
						l.Warnf("Warning: You have uncommitted changes. The --filter-affected flag may not include all your local modifications.")
					}

					defaultBranch := gitRunner.GetDefaultBranch(ctx, l)

					opts.FilterQueries = append(opts.FilterQueries, fmt.Sprintf("[%s...HEAD]", defaultBranch))

					return nil
				},
			},
		),
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        FilterAllowDestroyFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterAllowDestroyFlagName),
				Destination: &opts.FilterAllowDestroy,
				Usage:       "Allow destroy runs when using Git-based filters.",
			},
		),
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        FilterFileFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterFileFlagName),
				Destination: &opts.FiltersFile,
				Usage:       "Path to a file containing filter queries, one per line. Default is .terragrunt-filters.",
			},
		),
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        NoFilterFileFlagName,
				EnvVars:     tgPrefix.EnvVars(NoFilterFileFlagName),
				Destination: &opts.NoFiltersFile,
				Usage:       "Disable automatic reading of .terragrunt-filters file.",
			},
		),
	}
}
