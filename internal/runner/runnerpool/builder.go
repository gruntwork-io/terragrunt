package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.StackRunner, error) {
	// discovery configurations
	d := discovery.
		NewDiscovery(terragruntOptions.WorkingDir).
		WithDiscoverExternalDependencies().
		WithParseInclude().
		WithParseExclude().
		WithDiscoverDependencies().
		WithSuppressParseErrors().
		WithDiscoveryContext(&discovery.DiscoveryContext{Cmd: terragruntOptions.TerraformCommand})

	discovered, err := d.Discover(ctx, l, terragruntOptions)
	if err != nil {
		return nil, err
	}

	runner, err := NewRunnerPoolStack(l, terragruntOptions, discovered, opts...)
	if err != nil {
		return nil, err
	}

	return runner, nil
}
