// Package discoverysetup shares setup steps common to the discovery
// commands (find, list, browse) and the commands that act on every unit
// with --all.
package discoverysetup

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// checkout is how much of the repository each comparison worktree holds.
type checkout int

const (
	// checkoutForDiscovery holds only the directories the Git expressions
	// expand to when [FilteredPathsOnly] allows it, and everything otherwise.
	checkoutForDiscovery checkout = iota
	// checkoutEverything holds the whole repository, because the caller
	// parses the units it discovers inside the worktrees.
	checkoutEverything
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
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	d *discovery.Discovery,
) (*discovery.Discovery, func(context.Context), error) {
	return attachWorktrees(ctx, l, v, s, opts, d, checkoutForDiscovery)
}

// Units discovers the units under opts.WorkingDir that opts.Filters select,
// within opts.DiscoveryBoundary, sorted by path. Units of a Git filter come
// from comparison worktrees, so callers defer the returned cleanup
// immediately, even when an error is returned, and keep it deferred until
// they are done with the units.
func Units(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) (component.Components, func(context.Context), error) {
	d, err := discovery.NewFiltered(l, v.FS, discovery.FilteredOptions{
		WorkingDir:        opts.WorkingDir,
		DiscoveryBoundary: opts.DiscoveryBoundary,
		Filters:           opts.Filters,
	})
	if err != nil {
		return nil, noCleanup, err
	}

	d, cleanup, err := attachWorktrees(ctx, l, v, s, opts, d, checkoutEverything)
	if err != nil {
		return nil, cleanup, err
	}

	components, err := d.Discover(ctx, l, v, s, opts)
	if err != nil {
		return nil, cleanup, err
	}

	return components.Filter(component.UnitKind).Sort(), cleanup, nil
}

// FilteredPathsOnly reports whether the worktrees for filters can be checked
// out with only the directories the Git expressions in them expand to.
//
// They can when discovery reads nothing else. Parsing a configuration reads
// more, and so does another filter query: those reach the worktree discovery
// too, and can name a component the expansion never mentions.
func FilteredPathsOnly(d *discovery.Discovery, filters filter.Filters) bool {
	return !d.ParsesConfigs() && len(filters.ExcludingGitFilters()) == 0
}

func attachWorktrees(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	d *discovery.Discovery,
	c checkout,
) (*discovery.Discovery, func(context.Context), error) {
	wts, err := worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
		WorkingDir:        opts.WorkingDir,
		GitExpressions:    opts.Filters.UniqueGitFilters(),
		Experiments:       opts.Experiments,
		FilteredPathsOnly: c == checkoutForDiscovery && FilteredPathsOnly(d, opts.Filters),
	})
	if err != nil {
		return d, noCleanup, fmt.Errorf("failed to create worktrees: %w", err)
	}

	cleanup := func(ctx context.Context) {
		if cleanupErr := wts.Cleanup(ctx, l, v); cleanupErr != nil {
			l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
		}
	}

	if err := generate.WorktreeStacks(ctx, l, v, s, opts, wts); err != nil {
		return d, cleanup, err
	}

	return d.WithWorktrees(wts), cleanup, nil
}

// noCleanup is the cleanup for a setup that checked nothing out.
func noCleanup(context.Context) {}
