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
	"github.com/gruntwork-io/terragrunt/internal/report"
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
func resolveWorkingDir(tgOpts *options.TerragruntOptions) string {
	if tgOpts.RootWorkingDir != "" {
		return tgOpts.RootWorkingDir
	}

	return tgOpts.WorkingDir
}

// buildConfigFilenames returns the list of config filenames to consider, including custom if provided.
func buildConfigFilenames(tgOpts *options.TerragruntOptions) []string {
	configFilenames := append([]string{}, discovery.DefaultConfigFilenames...)
	customConfigName := filepath.Base(tgOpts.TerragruntConfigPath)
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

// extractReport finds ReportProvider in options and returns the report.
func extractReport(opts []common.Option) *report.Report {
	for _, opt := range opts {
		if rp, ok := opt.(common.ReportProvider); ok {
			return rp.GetReport()
		}
	}

	return nil
}

// newBaseDiscovery constructs the base discovery with common immutable options.
func newBaseDiscovery(
	tgOpts *options.TerragruntOptions,
	workingDir string,
	configFilenames []string,
	opts ...common.Option,
) *discovery.Discovery {
	anyOpts := make([]any, len(opts))
	for i, v := range opts {
		anyOpts[i] = v
	}

	d := discovery.
		NewDiscovery(workingDir).
		WithOptions(anyOpts...).
		WithConfigFilenames(configFilenames).
		WithRelationships().
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: workingDir,
			Cmd:        tgOpts.TerraformCliArgs.First(),
			Args:       tgOpts.TerraformCliArgs.Tail(),
		})

	return d
}

// prepareDiscovery constructs a configured discovery instance based on Terragrunt options and flags.
func prepareDiscovery(
	l log.Logger,
	tgOpts *options.TerragruntOptions,
	opts ...common.Option,
) (*discovery.Discovery, error) {
	workingDir := resolveWorkingDir(tgOpts)
	configFilenames := buildConfigFilenames(tgOpts)

	d := newBaseDiscovery(tgOpts, workingDir, configFilenames, opts...)

	// Apply filter queries when provided
	if len(tgOpts.FilterQueries) > 0 {
		filters, err := parseFilters(l, tgOpts.FilterQueries)
		if err != nil {
			return nil, errors.Errorf("failed to parse filter queries in %s: %w", workingDir, err)
		}

		d = d.WithFilters(filters)
	}

	// Apply worktrees for git filter expressions
	if w := extractWorktrees(opts); w != nil {
		d = d.WithWorktrees(w)
	}

	// Apply report for recording excluded external dependencies
	if r := extractReport(opts); r != nil {
		d = d.WithReport(r)
	}

	return d, nil
}

// discoverWithRetry runs discovery and retries without exclude-by-default if zero results
// are found and modules-that-include / units-reading flags are set.
func discoverWithRetry(
	ctx context.Context,
	l log.Logger,
	tgOpts *options.TerragruntOptions,
	opts ...common.Option,
) (component.Components, error) {
	// Initial discovery with current excludeByDefault setting
	d, err := prepareDiscovery(l, tgOpts, opts...)
	if err != nil {
		return nil, err
	}

	var discovered component.Components

	err = doWithTelemetry(ctx, telemetryDiscovery, map[string]any{
		"working_dir":       tgOpts.WorkingDir,
		"terraform_command": tgOpts.TerraformCommand,
	}, func(childCtx context.Context) error {
		var discoveryErr error

		discovered, discoveryErr = d.Discover(childCtx, l, tgOpts)
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
	tgOpts *options.TerragruntOptions,
	comps component.Components,
	opts ...common.Option,
) (common.StackRunner, error) {
	var runner common.StackRunner

	err := doWithTelemetry(ctx, telemetryCreation, map[string]any{
		"discovered_configs": len(comps),
		"terraform_command":  tgOpts.TerraformCommand,
	}, func(childCtx context.Context) error {
		var err2 error

		runner, err2 = NewRunnerPoolStack(childCtx, l, tgOpts, comps, opts...)

		return err2
	})
	if err != nil {
		return nil, err
	}

	return runner, nil
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
			return checkUnitVersionConstraints(
				checkCtx,
				l,
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
	unit *component.Unit,
) error {
	unitConfig := unit.Config()

	// This is almost definitely already parsed, but we'll check just in case.
	if unitConfig == nil {
		configCtx, pctx := config.NewParsingContext(ctx, l, unit.Execution.TerragruntOptions)
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

	if !unit.Execution.TerragruntOptions.TFPathExplicitlySet && unitConfig.TerraformBinary != "" {
		unit.Execution.TerragruntOptions.TFPath = unitConfig.TerraformBinary
	}

	if unit.Execution != nil && unit.Execution.Logger != nil {
		l = unit.Execution.Logger
	}

	_, err := run.PopulateTFVersion(ctx, l, unit.Execution.TerragruntOptions)
	if err != nil {
		return errors.Errorf("failed to populate Terraform version for unit %s: %w", unit.DisplayPath(), err)
	}

	terraformVersionConstraint := run.DefaultTerraformVersionConstraint
	if unitConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = unitConfig.TerraformVersionConstraint
	}

	if err := run.CheckTerraformVersion(terraformVersionConstraint, unit.Execution.TerragruntOptions); err != nil {
		return errors.Errorf("Terraform version check failed for unit %s: %w", unit.DisplayPath(), err)
	}

	if unitConfig.TerragruntVersionConstraint != "" {
		if err := run.CheckTerragruntVersion(
			unitConfig.TerragruntVersionConstraint,
			unit.Execution.TerragruntOptions,
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

			if _, exists := unit.Execution.TerragruntOptions.FeatureFlags.Load(flagName); !exists {
				unit.Execution.TerragruntOptions.FeatureFlags.Store(flagName, defaultValue)
			}
		}
	}

	return nil
}
