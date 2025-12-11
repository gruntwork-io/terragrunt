package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// telemetry event names used in this file
const (
	telemetryDiscovery      = "runner_pool_discovery"
	telemetryDiscoveryRetry = "runner_pool_discovery_retry"
	telemetryCreation       = "runner_pool_creation"
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
	isCustom := true

	for _, defaultName := range discovery.DefaultConfigFilenames {
		if customConfigName == defaultName {
			isCustom = false
			break
		}
	}

	if isCustom && customConfigName != "" && customConfigName != "." {
		configFilenames = append(configFilenames, customConfigName)
	}

	return configFilenames
}

// parseFilters wraps filter parsing for readability.
func parseFilters(queries []string) (filter.Filters, error) {
	if len(queries) == 0 {
		return filter.Filters{}, nil
	}

	return filter.ParseFilterQueries(queries)
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
	anyOpts := make([]interface{}, len(opts))
	for i, v := range opts {
		anyOpts[i] = v
	}

	d := discovery.
		NewDiscovery(workingDir).
		WithOptions(anyOpts...).
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithConfigFilenames(configFilenames).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: workingDir,
			Cmd:        tgOpts.TerraformCliArgs.First(),
			Args:       tgOpts.TerraformCliArgs.Tail(),
		})

	// Only include external dependencies in the run queue if explicitly requested via --queue-include-external.
	// This restores the pre-v0.94.0 behavior where external dependencies were excluded by default.
	// External dependencies are still detected and tracked for reporting purposes, but not fully discovered
	// unless this flag is set.
	// See: https://github.com/gruntwork-io/terragrunt/issues/5195
	if tgOpts.IncludeExternalDependencies {
		d = d.WithDiscoverExternalDependencies()
	}

	return d
}

// prepareDiscovery constructs a configured discovery instance based on Terragrunt options and flags.
func prepareDiscovery(
	tgOpts *options.TerragruntOptions,
	excludeByDefault bool,
	opts ...common.Option,
) (*discovery.Discovery, error) {
	workingDir := resolveWorkingDir(tgOpts)
	configFilenames := buildConfigFilenames(tgOpts)

	d := newBaseDiscovery(tgOpts, workingDir, configFilenames, opts...)

	// Include / exclude directories
	if len(tgOpts.IncludeDirs) > 0 {
		d = d.WithIncludeDirs(tgOpts.IncludeDirs)
	}

	if len(tgOpts.ExcludeDirs) > 0 {
		d = d.WithExcludeDirs(tgOpts.ExcludeDirs)
	}

	// Include behavior flags
	if tgOpts.StrictInclude {
		d = d.WithStrictInclude()
	}

	if excludeByDefault {
		d = d.WithExcludeByDefault()
	}

	// Enable reading file tracking when requested by CLI flags
	if len(tgOpts.ModulesThatInclude) > 0 || len(tgOpts.UnitsReading) > 0 {
		d = d.WithReadFiles()
	}

	// Apply filter queries when provided
	if len(tgOpts.FilterQueries) > 0 {
		filters, err := parseFilters(tgOpts.FilterQueries)
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
	d, err := prepareDiscovery(tgOpts, tgOpts.ExcludeByDefault, opts...)
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

	// Retry without exclude-by-default if no results and relevant flags are set
	if len(discovered) == 0 && (len(tgOpts.ModulesThatInclude) > 0 || len(tgOpts.UnitsReading) > 0) {
		l.Debugf("Runner pool discovery returned 0 configs; retrying without exclude-by-default")

		d, err = prepareDiscovery(tgOpts, false, opts...)
		if err != nil {
			return nil, err
		}

		err = doWithTelemetry(ctx, telemetryDiscoveryRetry, map[string]any{
			"working_dir": tgOpts.WorkingDir,
		}, func(childCtx context.Context) error {
			var retryErr error

			discovered, retryErr = d.Discover(childCtx, l, tgOpts)
			if retryErr == nil {
				l.Debugf("Runner pool retry discovery found %d configs", len(discovered))
			}

			return retryErr
		})
		if err != nil {
			return nil, err
		}
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
