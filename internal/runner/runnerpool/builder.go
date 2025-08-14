package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.StackRunner, error) {
	// discovery configurations
	d := discovery.
		NewDiscovery(terragruntOptions.WorkingDir).
		WithOptions(opts).
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithIncludeHiddenDirs([]string{config.StackDir}).
		WithDiscoveryContext(&discovery.DiscoveryContext{Cmd: terragruntOptions.TerraformCommand})

	// Only discover external dependencies when not explicitly excluded via flag/env.
	if terragruntOptions.IgnoreExternalDependencies {
		d = d.WithIgnoreExternalDependencies()
	} else {
		d = d.WithDiscoverExternalDependencies()
	}

	// Configure discovery to look for the configured Terragrunt file name if provided,
	// otherwise fall back to the default filename.
	filename := config.DefaultTerragruntConfigPath
	if terragruntOptions.TerragruntConfigPath != "" {
		filename = filepath.Base(terragruntOptions.TerragruntConfigPath)
	}

	d = d.WithConfigFilenames([]string{filename})

	// Apply include directory features based on terragrunt options
	if len(terragruntOptions.UnitsReading) > 0 {
		d = d.WithIncludeDirs(terragruntOptions.UnitsReading)
	}

	if len(terragruntOptions.ModulesThatInclude) > 0 {
		d = d.WithIncludeDirs(terragruntOptions.ModulesThatInclude)
	}

	if terragruntOptions.StrictInclude {
		d = d.WithStrictInclude()
	}

	if terragruntOptions.ExcludeByDefault {
		d = d.WithExcludeByDefault()
	}

	// Wrap discovery with telemetry
	var discovered discovery.DiscoveredConfigs

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
