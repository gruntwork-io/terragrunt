package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(
	ctx context.Context,
	l log.Logger,
	terragruntOptions *options.TerragruntOptions,
	opts ...common.Option,
) (common.StackRunner, map[string]*options.TerragruntOptions, map[string]log.Logger, error) {
	discovered, err := discoverWithRetry(ctx, l, terragruntOptions, opts...)
	if err != nil {
		return nil, nil, nil, err
	}

	runner, unitOpts, unitLoggers, err := createRunner(ctx, l, terragruntOptions, discovered, opts...)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := checkVersionConstraints(ctx, l, terragruntOptions, unitOpts, unitLoggers, runner.GetStack().Units); err != nil {
		return nil, nil, nil, err
	}

	return runner, unitOpts, unitLoggers, nil
}
