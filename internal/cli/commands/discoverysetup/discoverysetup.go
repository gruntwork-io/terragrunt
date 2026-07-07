// Package discoverysetup shares setup steps common to the discovery
// commands (find, list, browse).
package discoverysetup

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Worktrees creates comparison worktrees for the Git filter expressions on
// opts, generates the stacks inside them, and attaches them to d. Worktrees
// are created here instead of in the discovery constructor so that callers
// can defer cleanup in their own context: the returned cleanup logs its own
// failures and is safe to defer immediately, even when an error is returned,
// so worktrees left behind by a failed stack generation are still removed.
func Worktrees(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	d *discovery.Discovery,
) (*discovery.Discovery, func(context.Context), error) {
	wts, err := worktrees.NewWorktrees(ctx, l, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: opts.Filters.UniqueGitFilters(),
		Experiments:    opts.Experiments,
	})
	if err != nil {
		return d, func(context.Context) {}, fmt.Errorf("failed to create worktrees: %w", err)
	}

	cleanup := func(ctx context.Context) {
		if cleanupErr := wts.Cleanup(ctx, l); cleanupErr != nil {
			l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
		}
	}

	if err := generate.WorktreeStacks(ctx, l, v, opts, wts); err != nil {
		return d, cleanup, err
	}

	return d.WithWorktrees(wts), cleanup, nil
}
