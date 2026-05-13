package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Build stack runner using discovery and queueing mechanisms. v is the
// virtualized environment used by the per-unit version-constraint probe.
func Build(
	ctx context.Context,
	l log.Logger,
	v run.Venv,
	opts *options.TerragruntOptions,
	runnerOpts ...common.Option,
) (common.StackRunner, error) {
	discovered, err := discoverWithRetry(ctx, l, v.ToRoot(), opts, runnerOpts...)
	if err != nil {
		return nil, err
	}

	rnr, err := createRunner(ctx, l, opts, discovered, runnerOpts...)
	if err != nil {
		return nil, err
	}

	if err := checkVersionConstraints(ctx, l, v.ToRoot(), opts, rnr.GetStack().Units); err != nil {
		return nil, err
	}

	return rnr, nil
}
