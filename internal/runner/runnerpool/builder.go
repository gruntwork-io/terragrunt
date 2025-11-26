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
	// Prepare discovery
	d := prepareDiscovery(terragruntOptions, terragruntOptions.ExcludeByDefault, opts...)

	// Run discovery
	discovered, err := runDiscovery(ctx, l, d, terragruntOptions)
	if err != nil {
		return nil, err
	}

	// Optional retry path
	discovered, err = maybeRetryDiscovery(ctx, l, terragruntOptions, discovered, opts...)
	if err != nil {
		return nil, err
	}

	// Create the runner
	return createRunner(ctx, l, terragruntOptions, discovered, opts...)
}

// anySlice converts a slice of common.Option to a slice of interface{} for discovery plumbing.
func anySlice(in []common.Option) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}

	return out
}
