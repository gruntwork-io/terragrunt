package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(
	ctx context.Context,
	l log.Logger,
	terragruntOptions *options.TerragruntOptions,
	opts ...common.Option,
) (common.StackRunner, error) {
	// discovery configurations
	// Use RootWorkingDir which is the canonicalized absolute path, not WorkingDir which may be relative
	workingDir := terragruntOptions.RootWorkingDir
	if workingDir == "" {
		workingDir = terragruntOptions.WorkingDir
	}

	// Build config filenames list - include defaults plus any custom config file
	configFilenames := append([]string{}, discovery.DefaultConfigFilenames...)
	customConfigName := filepath.Base(terragruntOptions.TerragruntConfigPath)
	// Only add custom config if it's different from defaults
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
		WithOptions(opts...).
		WithDiscoverExternalDependencies().
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithConfigFilenames(configFilenames).
		WithDiscoveryContext(&component.DiscoveryContext{
			Cmd:  terragruntOptions.TerraformCliArgs.First(),
			Args: terragruntOptions.TerraformCliArgs.Tail(),
		})

	// Pass include directory filters to discovery
	// Discovery will use glob matching to filter units appropriately
	if len(terragruntOptions.IncludeDirs) > 0 {
		d = d.WithIncludeDirs(terragruntOptions.IncludeDirs)
	}

	// NOTE: We do NOT pass ExcludeDirs to discovery because excluded units need to be
	// discovered and reported (for --report-file functionality). The unit resolver will
	// handle exclusions after discovery, ensuring excluded units appear in reports.
	//
	// For now... We can probably use the following once runnerpool has been updated to not expect external
	// dependencies in the discovery results.
	//
	// if !terragruntOptions.IgnoreExternalDependencies {
	// 	d = d.WithDiscoverExternalDependencies()
	// }

	// Pass include behavior flags
	if terragruntOptions.StrictInclude {
		d = d.WithStrictInclude()
	}

	// Note: Discovery will use glob-based filtering for include patterns.
	// Exclude patterns are handled by the unit resolver to ensure proper reporting.

	// Apply filter queries if the filter-flag experiment is enabled
	if terragruntOptions.Experiments.Evaluate(experiment.FilterFlag) && len(terragruntOptions.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(terragruntOptions.FilterQueries, workingDir)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters)
	}

	// Wrap discovery with telemetry
	var discovered component.Components

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_discovery", map[string]any{
		"working_dir":       terragruntOptions.WorkingDir,
		"terraform_command": terragruntOptions.TerraformCommand,
	}, func(childCtx context.Context) error {
		var discoveryErr error

		discovered, discoveryErr = d.Discover(childCtx, l, terragruntOptions)

		return discoveryErr
	})
	if err != nil {
		return nil, err
	}

	// Wrap runner pool creation with telemetry
	var runner common.StackRunner

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_creation", map[string]any{
		"discovered_configs": len(discovered),
		"terraform_command":  terragruntOptions.TerraformCommand,
	}, func(childCtx context.Context) error {
		var runnerErr error

		runner, runnerErr = NewRunnerPoolStack(childCtx, l, terragruntOptions, discovered, opts...)

		return runnerErr
	})
	if err != nil {
		return nil, err
	}

	return runner, nil
}
