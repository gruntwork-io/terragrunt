package runner

import (
	"context"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// New discovers all Terragrunt units under the working directory and assembles them
// into a [Runner] that can apply or destroy them.
func New(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	runnerOpts ...Option,
) (*Runner, error) {
	discovered, err := discoverWithRetry(ctx, l, v, s, opts, runnerOpts...)
	if err != nil {
		return nil, err
	}

	rnr, err := createRunner(ctx, l, opts, discovered)
	if err != nil {
		return nil, err
	}

	if err := checkVersionConstraints(ctx, l, v, opts, rnr.GetStack().Units); err != nil {
		return nil, err
	}

	return rnr, nil
}
