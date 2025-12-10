package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(
	ctx context.Context,
	l log.Logger,
	terragruntOptions *options.TerragruntOptions,
	opts ...common.Option,
) (common.StackRunner, error) {
	// Run discovery (with automatic retry if needed)
	discovered, externalDeps, err := discoverWithRetry(ctx, l, terragruntOptions, opts...)
	if err != nil {
		return nil, err
	}

	// Create the runner with external dependencies for reporting
	return createRunner(ctx, l, terragruntOptions, discovered, externalDeps, opts...)
}
