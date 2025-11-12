package discovery

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// WorktreeDiscovery is the configuration for discovery in Git worktrees.
type WorktreeDiscovery struct {
	// discoveryContext is the context in which discovery was originally triggered.
	discoveryContext *component.DiscoveryContext

	// originalDiscovery is the original discovery object that triggered the worktree discovery.
	originalDiscovery *Discovery

	// workTrees is a map of references to the worktrees created for Git-based filters.
	workTrees map[string]string

	// gitExpressions contains Git filter expressions that require worktree discovery
	gitExpressions filter.GitFilters

	// numWorkers is the number of workers to use to discover in worktrees.
	numWorkers int
}

// NewWorktreeDiscovery creates a new WorktreeDiscovery with the given configuration.
func NewWorktreeDiscovery(gitExpressions filter.GitFilters, workTrees map[string]string) *WorktreeDiscovery {
	return &WorktreeDiscovery{
		gitExpressions: gitExpressions,
		workTrees:      workTrees,
		numWorkers:     runtime.NumCPU(),
	}
}

// WithNumWorkers sets the number of workers for worktree discovery.
func (wd *WorktreeDiscovery) WithNumWorkers(numWorkers int) *WorktreeDiscovery {
	wd.numWorkers = numWorkers
	return wd
}

// WithDiscoveryContext sets the discovery context in which discovery was originally triggered.
func (wd *WorktreeDiscovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *WorktreeDiscovery {
	wd.discoveryContext = discoveryContext
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
) (component.Components, map[string]string, error) {
	gitRefs := wd.gitExpressions.UniqueGitRefs()
	if len(gitRefs) > 0 {
		if worktreeErr := wd.createGitWorktrees(ctx, l); worktreeErr != nil {
			return nil, wd.workTrees, worktreeErr
		}
	}

	var (
		errs []error
		mu   sync.Mutex
	)

	expressionToDiffs := make(map[*filter.GitFilter]*git.Diffs, len(wd.gitExpressions))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(wd.numWorkers)

	for _, gitExpression := range wd.gitExpressions {
		g.Go(func() error {
			gitRunner, err := git.NewGitRunner()
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()

				return nil
			}

			gitRunner = gitRunner.WithWorkDir(wd.discoveryContext.WorkingDir)

			diffs, err := gitRunner.Diff(ctx, gitExpression.FromRef, gitExpression.ToRef)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()

				return nil
			}

			expressionToDiffs[gitExpression] = diffs

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, wd.workTrees, err
	}

	if len(errs) > 0 {
		return nil, wd.workTrees, errors.Join(errs...)
	}

	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	for gitExpression, diffs := range expressionToDiffs {
		fromExpressions, toExpressions, err := gitExpression.Expand(diffs)
		if err != nil {
			return nil, wd.workTrees, err
		}

		fromDiscovery := *wd.originalDiscovery

		fromDiscoveryContext := *fromDiscovery.discoveryContext
		fromDiscoveryContext.Ref = gitExpression.FromRef
		fromDiscoveryContext.WorkingDir = wd.workTrees[gitExpression.FromRef]

		switch {
		case (fromDiscoveryContext.Cmd == "plan" || fromDiscoveryContext.Cmd == "apply") &&
			!slices.Contains(fromDiscoveryContext.Args, "-destroy"):

			fromDiscoveryContext.Args = append(fromDiscoveryContext.Args, "-destroy")
		case (fromDiscoveryContext.Cmd == "" && len(fromDiscoveryContext.Args) == 0):
			// This is the case when using a discovery command like find or list.
			// It's fine for these commands to not have any command or arguments.
		default:
			return nil, wd.workTrees, NewGitFilterCommandError(fromDiscoveryContext.Cmd, fromDiscoveryContext.Args)
		}

		components, err := fromDiscovery.
			WithFilters(fromExpressions).
			WithDiscoveryContext(&fromDiscoveryContext).
			Discover(ctx, l, opts)
		if err != nil {
			return nil, wd.workTrees, err
		}

		for _, component := range components {
			discoveredComponents.EnsureComponent(component)
		}

		toDiscovery := *wd.originalDiscovery

		toDiscoveryContext := *toDiscovery.discoveryContext
		toDiscoveryContext.Ref = gitExpression.ToRef
		toDiscoveryContext.WorkingDir = wd.workTrees[gitExpression.ToRef]

		switch {
		case (fromDiscoveryContext.Cmd == "plan" || fromDiscoveryContext.Cmd == "apply") &&
			!slices.Contains(fromDiscoveryContext.Args, "-destroy"):
		case (fromDiscoveryContext.Cmd == "" && len(fromDiscoveryContext.Args) == 0):
			// This is the case when using a discovery command like find or list.
			// It's fine for these commands to not have any command or arguments.
		default:
			return nil, wd.workTrees, NewGitFilterCommandError(fromDiscoveryContext.Cmd, fromDiscoveryContext.Args)
		}

		toComponents, err := toDiscovery.
			WithFilters(toExpressions).
			WithDiscoveryContext(&toDiscoveryContext).
			Discover(ctx, l, opts)
		if err != nil {
			return nil, wd.workTrees, err
		}

		for _, component := range toComponents {
			discoveredComponents.EnsureComponent(component)
		}
	}

	return discoveredComponents.ToComponents(), wd.workTrees, nil
}

// createGitWorktrees creates detached worktrees for each unique Git reference needed by filters.
// The worktrees are created in temporary directories and tracked in d.gitWorktrees.
func (wd *WorktreeDiscovery) createGitWorktrees(ctx context.Context, l log.Logger) error {
	gitRefs := wd.gitExpressions.UniqueGitRefs()
	if len(gitRefs) == 0 {
		return nil
	}

	gitRunner, err := git.NewGitRunner()
	if err != nil {
		return errors.New(err)
	}

	gitRunner = gitRunner.WithWorkDir(wd.discoveryContext.WorkingDir)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(wd.numWorkers)

	var (
		errs []error
		mu   sync.Mutex
	)

	wd.workTrees = make(map[string]string, len(gitRefs))

	for _, ref := range gitRefs {
		g.Go(func() error {
			tempDir, err := os.MkdirTemp("", "terragrunt-worktree-"+sanitizeRef(ref)+"-*")
			if err != nil {
				mu.Lock()

				errs = append(errs, fmt.Errorf("failed to create temporary directory for worktree: %w", err))

				mu.Unlock()

				return nil
			}

			err = gitRunner.CreateDetachedWorktree(ctx, tempDir, ref)
			if err != nil {
				mu.Lock()

				errs = append(errs, fmt.Errorf("failed to create Git worktree for reference %s: %w", ref, err))

				mu.Unlock()

				return nil
			}

			mu.Lock()

			wd.workTrees[ref] = tempDir

			mu.Unlock()

			l.Debugf("Created Git worktree for reference %s at %s", ref, tempDir)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
