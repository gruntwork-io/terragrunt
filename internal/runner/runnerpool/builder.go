package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// discoveryUnitFilterAdapter adapts common.UnitFilter to discovery.UnitFilter interface
type discoveryUnitFilterAdapter struct {
	filter common.UnitFilter
}

func (a *discoveryUnitFilterAdapter) Filter(ctx context.Context, units component.Units, opts *options.TerragruntOptions) error {
	// common.UnitFilter and discovery.UnitFilter have the same signature
	return a.filter.Filter(ctx, units, opts)
}

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

	// Create discovery with options and report for integrated unit resolution
	// NOTE: Need to extract report from opts first
	var reportForDiscovery *report.Report
	var unitFilters []discovery.UnitFilter

	// Apply options to extract report and filters
	for _, opt := range opts {
		// Apply to a temporary runner to extract fields
		tempStack := component.NewStack("")
		tempRunner := &Runner{Stack: tempStack}
		tempRunner = tempRunner.WithOptions(opt)
		if tempRunner.Stack.Report() != nil {
			reportForDiscovery = tempRunner.Stack.Report()
		}
		// Extract unit filters if any
		if len(tempRunner.unitFilters) > 0 {
			// Convert common.UnitFilter to discovery.UnitFilter
			for _, f := range tempRunner.unitFilters {
				// Wrap the filter to match discovery's UnitFilter interface
				unitFilters = append(unitFilters, &discoveryUnitFilterAdapter{filter: f})
			}
		}
	}

	d := discovery.
		NewDiscovery(workingDir).
		WithContext(ctx).
		WithTerragruntOptions(terragruntOptions).
		WithReport(reportForDiscovery).
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

	// Pass unit filters to discovery
	if len(unitFilters) > 0 {
		d = d.WithUnitFilters(unitFilters...)
	}

	// Pass include directory filters to discovery
	// Discovery will now handle filtering and reporting
	if len(terragruntOptions.IncludeDirs) > 0 {
		d = d.WithIncludeDirs(terragruntOptions.IncludeDirs)
	}

	// Pass exclude directory filters to discovery
	// Discovery will handle exclusions and ensure excluded units appear in reports
	if len(terragruntOptions.ExcludeDirs) > 0 {
		d = d.WithExcludeDirs(terragruntOptions.ExcludeDirs)
	}

	// Pass include behavior flags
	if terragruntOptions.StrictInclude {
		d = d.WithStrictInclude()
	}

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
