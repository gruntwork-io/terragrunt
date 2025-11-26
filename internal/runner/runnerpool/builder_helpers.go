package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// prepareDiscovery constructs a configured discovery instance based on Terragrunt options and flags.
func prepareDiscovery(
	tgOpts *options.TerragruntOptions,
	excludeByDefault bool,
	opts ...common.Option,
) *discovery.Discovery {
	// Determine the canonical working directory to use for discovery
	workingDir := tgOpts.RootWorkingDir
	if workingDir == "" {
		workingDir = tgOpts.WorkingDir
	}

	// Build config filenames list - include defaults plus any custom config file
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

	d := discovery.
		NewDiscovery(workingDir).
		WithOptions(anySlice(opts)...).
		WithDiscoverExternalDependencies().
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithConfigFilenames(configFilenames).
		WithDiscoveryContext(&component.DiscoveryContext{
			Cmd:  tgOpts.TerraformCliArgs.First(),
			Args: tgOpts.TerraformCliArgs.Tail(),
		})

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
		if filters, err := filter.ParseFilterQueries(tgOpts.FilterQueries, workingDir); err == nil {
			d = d.WithFilters(filters)
		}
	}

	return d
}

// runDiscovery executes discovery with telemetry and returns discovered components.
func runDiscovery(
	ctx context.Context,
	l log.Logger,
	d *discovery.Discovery,
	tgOpts *options.TerragruntOptions,
) (component.Components, error) {
	var discovered component.Components

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_discovery", map[string]any{
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

// maybeRetryDiscovery retries discovery without exclude-by-default if none were discovered
// and modules-that-include / units-reading flags are set.
func maybeRetryDiscovery(
	ctx context.Context,
	l log.Logger,
	tgOpts *options.TerragruntOptions,
	discovered component.Components,
	opts ...common.Option,
) (component.Components, error) {
	if len(discovered) > 0 {
		return discovered, nil
	}

	if len(tgOpts.ModulesThatInclude) == 0 && len(tgOpts.UnitsReading) == 0 {
		return discovered, nil
	}

	l.Debugf("Runner pool discovery returned 0 configs with modules-that-include; retrying without exclude-by-default")

	disc := prepareDiscovery(tgOpts, false, opts...)

	var retryErr error

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_discovery_retry", map[string]any{
		"working_dir": tgOpts.WorkingDir,
	}, func(childCtx context.Context) error {
		discovered, retryErr = disc.Discover(childCtx, l, tgOpts)
		if retryErr == nil {
			l.Debugf("Runner pool retry discovery found %d configs", len(discovered))
		}

		return retryErr
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

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_creation", map[string]any{
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
