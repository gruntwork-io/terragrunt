package discovery

import (
	"context"
	"runtime"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// WorktreeDiscovery is the configuration for discovery in Git worktrees.
type WorktreeDiscovery struct {
	// originalDiscovery is the original discovery object that triggered the worktree discovery.
	originalDiscovery *Discovery

	// gitExpressions contains Git filter expressions that require worktree discovery
	gitExpressions filter.GitExpressions

	// numWorkers is the number of workers to use to discover in worktrees.
	numWorkers int
}

// NewWorktreeDiscovery creates a new WorktreeDiscovery with the given configuration.
func NewWorktreeDiscovery(gitExpressions filter.GitExpressions) *WorktreeDiscovery {
	return &WorktreeDiscovery{
		gitExpressions: gitExpressions,
		numWorkers:     runtime.NumCPU(),
	}
}

// WithNumWorkers sets the number of workers for worktree discovery.
func (wd *WorktreeDiscovery) WithNumWorkers(numWorkers int) *WorktreeDiscovery {
	wd.numWorkers = numWorkers
	return wd
}

// WithOriginalDiscovery sets the original discovery object that triggered the worktree discovery.
func (wd *WorktreeDiscovery) WithOriginalDiscovery(originalDiscovery *Discovery) *WorktreeDiscovery {
	wd.originalDiscovery = originalDiscovery
	return wd
}

// Discover discovers components in all worktrees.
func (wd *WorktreeDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	worktree *worktrees.Worktrees,
) (component.Components, error) {
	if worktree == nil {
		l.Debug("No worktrees provided, skipping worktree discovery")

		return component.Components{}, nil
	}

	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	// Run from and to discovery concurrently for each expression
	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(wd.numWorkers)

	for gitExpression, diffs := range worktree.GitExpressionsToDiffs {
		discoveryGroup.Go(func() error {
			fromFilters, toFilters, err := gitExpression.Expand(diffs)
			if err != nil {
				return err
			}

			// Run from and to discovery concurrently
			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			// We only kick off from/to discovery if there any filters expanded from the git expression.
			// This ensures that we don't discover anything when using a Git filter that doesn't match anything.

			if len(fromFilters) > 0 {
				fromToG.Go(func() error {
					fromDiscovery := *wd.originalDiscovery

					fromDiscoveryContext := *fromDiscovery.discoveryContext
					fromDiscoveryContext.Ref = gitExpression.FromRef
					fromDiscoveryContext.WorkingDir = worktree.RefsToPaths[gitExpression.FromRef]

					switch {
					case (fromDiscoveryContext.Cmd == "plan" || fromDiscoveryContext.Cmd == "apply") &&
						!slices.Contains(fromDiscoveryContext.Args, "-destroy"):
						fromDiscoveryContext.Args = append(fromDiscoveryContext.Args, "-destroy")
					case (fromDiscoveryContext.Cmd == "" && len(fromDiscoveryContext.Args) == 0):
						// This is the case when using a discovery command like find or list.
						// It's fine for these commands to not have any command or arguments.
					default:
						return NewGitFilterCommandError(fromDiscoveryContext.Cmd, fromDiscoveryContext.Args)
					}

					components, err := fromDiscovery.
						WithFilters(fromFilters).
						WithDiscoveryContext(&fromDiscoveryContext).
						Discover(fromToCtx, l, opts)
					if err != nil {
						return err
					}

					for _, component := range components {
						discoveredComponents.EnsureComponent(component)
					}

					return nil
				})
			}

			if len(toFilters) > 0 {
				fromToG.Go(func() error {
					toDiscovery := *wd.originalDiscovery

					toDiscoveryContext := *toDiscovery.discoveryContext
					toDiscoveryContext.Ref = gitExpression.ToRef
					toDiscoveryContext.WorkingDir = worktree.RefsToPaths[gitExpression.ToRef]

					switch {
					case (toDiscoveryContext.Cmd == "plan" || toDiscoveryContext.Cmd == "apply") &&
						!slices.Contains(toDiscoveryContext.Args, "-destroy"):
					case (toDiscoveryContext.Cmd == "" && len(toDiscoveryContext.Args) == 0):
						// This is the case when using a discovery command like find or list.
						// It's fine for these commands to not have any command or arguments.
					default:
						return NewGitFilterCommandError(toDiscoveryContext.Cmd, toDiscoveryContext.Args)
					}

					toComponents, err := toDiscovery.
						WithFilters(toFilters).
						WithDiscoveryContext(&toDiscoveryContext).
						Discover(fromToCtx, l, opts)
					if err != nil {
						return err
					}

					for _, component := range toComponents {
						discoveredComponents.EnsureComponent(component)
					}

					return nil
				})
			}

			return fromToG.Wait()
		})
	}

	if err := discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	return discoveredComponents.ToComponents(), nil
}
