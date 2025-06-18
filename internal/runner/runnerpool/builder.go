package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Build discovers modules and builds a new DefaultStack, returning it as a Stack interface.
func Build(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.Stack, error) {
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

	// build processing queue for discovered configurations
	q, queueErr := queue.NewQueue(discovered)
	if queueErr != nil {
		return nil, queueErr
	}

	return NewRunnerPoolStack(ctx, l, terragruntOptions, q.Configs())
}
