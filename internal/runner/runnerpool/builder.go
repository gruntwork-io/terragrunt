package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Build stack runner using discovery and queueing mechanisms.
func Build(
	ctx context.Context,
	l log.Logger,
	terragruntOptions *options.TerragruntOptions,
	rpt *report.Report,
	opts ...common.Option,
) (common.StackRunner, error) {
	discovered, err := discoverWithRetry(ctx, l, terragruntOptions, rpt, opts...)
	if err != nil {
		return nil, err
	}

	runner, err := createRunner(ctx, l, terragruntOptions, discovered, opts...)
	if err != nil {
		return nil, err
	}

	if err := checkVersionConstraints(ctx, l, terragruntOptions, runner.GetStack().Units); err != nil {
		return nil, err
	}

	return runner, nil
}
