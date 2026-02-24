package runnerpool

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// telemetry event names used in this file
const (
	telemetryDiscovery = "runner_pool_discovery"
	telemetryCreation  = "runner_pool_creation"
)

// doWithTelemetry is a small helper to standardize telemetry collection calls.
func doWithTelemetry(ctx context.Context, name string, fields map[string]any, fn func(context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, name, fields, fn)
}

// resolveWorkingDir determines the canonical working directory for discovery.
func resolveWorkingDir(opts *options.TerragruntOptions) string {
	if opts.RootWorkingDir != "" {
		return opts.RootWorkingDir
	}

	return opts.WorkingDir
}

// buildConfigFilenames returns the list of config filenames to consider, including custom if provided.
func buildConfigFilenames(opts *options.TerragruntOptions) []string {
	configFilenames := append([]string{}, discovery.DefaultConfigFilenames...)
	customConfigName := filepath.Base(opts.TerragruntConfigPath)
	isCustom := !slices.Contains(discovery.DefaultConfigFilenames, customConfigName)

	if isCustom && customConfigName != "" && customConfigName != "." {
		configFilenames = append(configFilenames, customConfigName)
	}

	return configFilenames
}

// parseFilters wraps filter parsing for readability.
func parseFilters(l log.Logger, queries []string) (filter.Filters, error) {
	if len(queries) == 0 {
		return filter.Filters{}, nil
	}

	return filter.ParseFilterQueries(l, queries)
}

// extractWorktrees finds WorktreeOption in options and returns worktrees.
func extractWorktrees(opts []common.Option) *worktrees.Worktrees {
	for _, opt := range opts {
		if wo, ok := opt.(common.WorktreeOption); ok {
			return wo.Worktrees
		}
	}

	return nil
}

// newBaseDiscovery constructs the base discovery with common immutable options.
func newBaseDiscovery(
	opts *options.TerragruntOptions,
	workingDir string,
	configFilenames []string,
	runnerOpts ...common.Option,
) *discovery.Discovery {
	anyOpts := make([]any, len(runnerOpts))
	for i, v := range runnerOpts {
		anyOpts[i] = v
	}

	d := discovery.
		NewDiscovery(workingDir).
		WithOptions(anyOpts...).
		WithConfigFilenames(configFilenames).
		WithRelationships().
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: workingDir,
			Cmd:        opts.TerraformCliArgs.First(),
			Args:       opts.TerraformCliArgs.Tail(),
		})

	return d
}

// prepareDiscovery constructs a configured discovery instance based on Terragrunt options and flags.
func prepareDiscovery(
	l log.Logger,
	opts *options.TerragruntOptions,
	runnerOpts ...common.Option,
) (*discovery.Discovery, error) {
	workingDir := resolveWorkingDir(opts)
	configFilenames := buildConfigFilenames(opts)

	d := newBaseDiscovery(opts, workingDir, configFilenames, runnerOpts...)

	// Apply filter queries when provided
	if len(opts.FilterQueries) > 0 {
		filters, err := parseFilters(l, opts.FilterQueries)
		if err != nil {
			return nil, errors.Errorf("failed to parse filter queries in %s: %w", workingDir, err)
		}

		d = d.WithFilters(filters)
	}

	// Apply worktrees for git filter expressions
	if w := extractWorktrees(runnerOpts); w != nil {
		d = d.WithWorktrees(w)
	}

	return d, nil
}

// discoverWithRetry runs discovery and retries without exclude-by-default if zero results
// are found and modules-that-include / units-reading flags are set.
func discoverWithRetry(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	runnerOpts ...common.Option,
) (component.Components, error) {
	// Initial discovery with current excludeByDefault setting
	d, err := prepareDiscovery(l, opts, runnerOpts...)
	if err != nil {
		return nil, err
	}

	var discovered component.Components

	err = doWithTelemetry(ctx, telemetryDiscovery, map[string]any{
		"working_dir":       opts.WorkingDir,
		"terraform_command": opts.TerraformCommand,
	}, func(childCtx context.Context) error {
		var discoveryErr error

		discovered, discoveryErr = d.Discover(childCtx, l, opts)
		if discoveryErr == nil {
			l.Debugf("Runner pool discovery found %d configs", len(discovered))
		}

		return discoveryErr
	})
	if err != nil {
		return nil, err
	}

	return discovered, nil
}

// createRunner wraps runner creation with telemetry and returns the stack runner.
func createRunner(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	comps component.Components,
	runnerOpts ...common.Option,
) (common.StackRunner, error) {
	var rnr common.StackRunner

	err := doWithTelemetry(ctx, telemetryCreation, map[string]any{
		"discovered_configs": len(comps),
		"terraform_command":  opts.TerraformCommand,
	}, func(childCtx context.Context) error {
		var err2 error

		rnr, err2 = NewRunnerPoolStack(childCtx, l, opts, comps, runnerOpts...)

		return err2
	})
	if err != nil {
		return nil, err
	}

	return rnr, nil
}

// checkVersionConstraints performs version constraint checks on all discovered units concurrently.
// It uses errgroup to coordinate concurrent checks and returns the first error encountered.
func checkVersionConstraints(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	units []*component.Unit,
) error {
	g, checkCtx := errgroup.WithContext(ctx)

	maxWorkers := min(runtime.NumCPU(), opts.Parallelism)
	g.SetLimit(maxWorkers)

	for _, unit := range units {
		g.Go(func() error {
			unitOpts, unitLogger, err := BuildUnitOpts(l, opts, unit)
			if err != nil {
				return err
			}

			return checkUnitVersionConstraints(
				checkCtx,
				l,
				unitOpts,
				unitLogger,
				unit,
			)
		})
	}

	return g.Wait()
}

// checkUnitVersionConstraints checks version constraints for a single unit.
// It handles config parsing if needed and performs version constraint validation.
func checkUnitVersionConstraints(
	ctx context.Context,
	l log.Logger,
	unitOpts *options.TerragruntOptions,
	unitLogger log.Logger,
	unit *component.Unit,
) error {
	unitConfig := unit.Config()

	// This is almost definitely already parsed, but we'll check just in case.
	if unitConfig == nil {
		configCtx, pctx := config.NewParsingContext(ctx, l, unitOpts)
		pctx = pctx.WithDecodeList(
			config.TerragruntVersionConstraints,
			config.FeatureFlagsBlock,
		)

		var err error

		unitConfig, err = config.PartialParseConfigFile(
			configCtx,
			pctx,
			l,
			unit.ConfigFile(),
			nil,
		)
		if err != nil {
			return errors.Errorf("failed to parse config for unit %s: %w", unit.DisplayPath(), err)
		}
	}

	if !unitOpts.TFPathExplicitlySet && unitConfig.TerraformBinary != "" {
		unitOpts.TFPath = unitConfig.TerraformBinary
	}

	if unitLogger != nil {
		l = unitLogger
	}

	_, err := run.PopulateTFVersion(ctx, l, unitOpts)
	if err != nil {
		return errors.Errorf("failed to populate Terraform version for unit %s: %w", unit.DisplayPath(), err)
	}

	terraformVersionConstraint := run.DefaultTerraformVersionConstraint
	if unitConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = unitConfig.TerraformVersionConstraint
	}

	if err := run.CheckTerraformVersion(terraformVersionConstraint, unitOpts); err != nil {
		return errors.Errorf("Terraform version check failed for unit %s: %w", unit.DisplayPath(), err)
	}

	if unitConfig.TerragruntVersionConstraint != "" {
		if err := run.CheckTerragruntVersion(
			unitConfig.TerragruntVersionConstraint,
			unitOpts,
		); err != nil {
			return errors.Errorf("Terragrunt version check failed for unit %s: %w", unit.DisplayPath(), err)
		}
	}

	if unitConfig.FeatureFlags != nil {
		for _, flag := range unitConfig.FeatureFlags {
			flagName := flag.Name

			defaultValue, err := flag.DefaultAsString()
			if err != nil {
				return errors.Errorf("failed to get default value for feature flag %s in unit %s: %w", flagName, unit.DisplayPath(), err)
			}

			if _, exists := unitOpts.FeatureFlags.Load(flagName); !exists {
				unitOpts.FeatureFlags.Store(flagName, defaultValue)
			}
		}
	}

	return nil
}
