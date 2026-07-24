package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Build stack runner using discovery and queueing mechanisms. v is the
// virtualized environment used by the per-unit version-constraint probe.
func Build(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	runnerOpts ...common.Option,
) (common.StackRunner, error) {
	discovered, err := discoverWithRetry(ctx, l, v, opts, runnerOpts...)
	if err != nil {
		return nil, err
	}

	rnr, err := createRunner(ctx, l, opts, discovered, runnerOpts...)
	if err != nil {
		return nil, err
	}

	if err := checkVersionConstraints(ctx, l, v, opts, rnr.GetStack().Units); err != nil {
		return nil, err
	}

	return rnr, nil
}
