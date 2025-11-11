package discovery

import (
	"context"
	"runtime"
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
	// discoveredComponents is a thread-safe collection of components that have been discovered.
	discoveredComponents *component.ThreadSafeComponents

	// discoveryContext is the context in which discovery was originally triggered.
	discoveryContext *component.DiscoveryContext

	// workTrees is a map of references to the worktrees created for Git-based filters.
	workTrees map[string]string

	// gitExpressions contains Git filter expressions that require worktree discovery
	gitExpressions filter.GitFilters

	// workingDir is the working directory initially used for discovery.
	workingDir string

	// numWorkers is the number of workers to use to discover in worktrees.
	numWorkers int
}

// NewWorktreeDiscovery creates a new WorktreeDiscovery with the given configuration.
func NewWorktreeDiscovery(gitExpressions filter.GitFilters, workTrees map[string]string) *WorktreeDiscovery {
	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	return &WorktreeDiscovery{
		discoveredComponents: discoveredComponents,
		gitExpressions:       gitExpressions,
		workTrees:            workTrees,
		numWorkers:           runtime.NumCPU(),
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

// WithWorkingDir sets the working directory initially used for discovery.
func (wd *WorktreeDiscovery) WithWorkingDir(workingDir string) *WorktreeDiscovery {
	wd.workingDir = workingDir
	return wd
}

// Discover discovers components in all worktrees.
func (wd *WorktreeDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (component.Components, error) {
	diffsToCheck := make(map[string]string, len(wd.gitExpressions))

	// We do this to deduplicate multiple calls to `git diff` for the same
	// Git reference pair.
	for _, diff := range wd.gitExpressions {
		fromRef := diff.FromRef
		toRef := diff.ToRef
		if toRef == "" {
			toRef = "HEAD"
		}

		diffsToCheck[fromRef] = toRef
	}

	var (
		errs []error
		mu   sync.Mutex
	)

	expressionToDiffs := make(map[string]*git.Diffs, len(wd.gitExpressions))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(wd.numWorkers)

	for from, to := range diffsToCheck {
		g.Go(func() error {
			gitRunner, err := git.NewGitRunner()
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()

				return nil
			}

			gitRunner = gitRunner.WithWorkDir(wd.workingDir)

			diffs, err := gitRunner.Diff(ctx, from, to)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()

				return nil
			}

			expressionToDiffs[from+"..."+to] = diffs

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return nil, nil
}
