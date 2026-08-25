package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// MaxDependencyTraversalDepth exposes the dependency traversal bound to tests.
const MaxDependencyTraversalDepth = maxDependencyTraversalDepth

// CheckVersionConstraints exposes checkVersionConstraints to tests.
func CheckVersionConstraints(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	units []*component.Unit,
) error {
	return checkVersionConstraints(ctx, l, v, opts, units)
}
