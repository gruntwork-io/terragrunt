package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
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
	d := discovery.
		NewDiscovery(terragruntOptions.WorkingDir).
		WithOptions(opts...).
		WithDiscoverExternalDependencies().
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithConfigFilenames([]string{filepath.Base(terragruntOptions.TerragruntConfigPath)}).
		WithDiscoveryContext(&discovery.DiscoveryContext{
			Cmd:  terragruntOptions.TerraformCliArgs.First(),
			Args: terragruntOptions.TerraformCliArgs.Tail(),
		})

	// Pass include/exclude directory filters
	if len(terragruntOptions.IncludeDirs) > 0 {
		d = d.WithIncludeDirs(terragruntOptions.IncludeDirs)
	}

	if len(terragruntOptions.ExcludeDirs) > 0 {
		d = d.WithExcludeDirs(terragruntOptions.ExcludeDirs)
	}

	// Pass include behavior flags
	if terragruntOptions.StrictInclude {
		d = d.WithStrictInclude()
	}

	if terragruntOptions.ExcludeByDefault {
		d = d.WithExcludeByDefault()
	}

	// Pass dependency behavior flags
	if terragruntOptions.IgnoreExternalDependencies {
		d = d.WithIgnoreExternalDependencies()
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
