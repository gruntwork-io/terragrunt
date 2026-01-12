package discovery

import (
	"context"
	"runtime"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"golang.org/x/sync/errgroup"
)

// DependencyDiscovery is the configuration for a DependencyDiscovery.
type DependencyDiscovery struct {
	components          *component.ThreadSafeComponents
	report              *report.Report
	mu                  *sync.RWMutex
	seenComponents      map[string]struct{}
	parserOptions       []hclparse.Option
	maxDepth            int
	numWorkers          int
	suppressParseErrors bool
}

func NewDependencyDiscovery(components *component.ThreadSafeComponents) *DependencyDiscovery {
	return &DependencyDiscovery{
		components:     components,
		mu:             &sync.RWMutex{},
		seenComponents: make(map[string]struct{}),
		numWorkers:     runtime.NumCPU(),
	}
}

// WithMaxDepth sets the maximum depth for dependency discovery.
func (dd *DependencyDiscovery) WithMaxDepth(maxDepth int) *DependencyDiscovery {
	dd.maxDepth = maxDepth
	return dd
}

// WithNumWorkers sets the number of workers for dependency discovery.
func (dd *DependencyDiscovery) WithNumWorkers(numWorkers int) *DependencyDiscovery {
	dd.numWorkers = numWorkers
	return dd
}

// WithSuppressParseErrors sets the SuppressParseErrors flag to true.
func (dd *DependencyDiscovery) WithSuppressParseErrors() *DependencyDiscovery {
	dd.suppressParseErrors = true

	return dd
}

// WithParserOptions sets custom HCL parser options for dependency discovery.
func (dd *DependencyDiscovery) WithParserOptions(options []hclparse.Option) *DependencyDiscovery {
	dd.parserOptions = options
	return dd
}

// WithReport sets the report for recording excluded external dependencies.
func (dd *DependencyDiscovery) WithReport(r *report.Report) *DependencyDiscovery {
	dd.report = r

	return dd
}

func (dd *DependencyDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	startingComponents component.Components,
) error {
	var (
		errs []error
		mu   sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(dd.numWorkers)

	for _, c := range startingComponents {
		dd.markSeen(c.Path())

		if _, ok := c.(*component.Stack); ok {
			continue
		}

		g.Go(func() error {
			err := dd.discoverDependencies(ctx, l, opts, c, dd.maxDepth)
			if err != nil {
				mu.Lock()

				errs = append(errs, errors.New(err))

				mu.Unlock()
			}

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

// markSeen marks a component path as seen.
func (dd *DependencyDiscovery) markSeen(path string) {
	dd.mu.Lock()
	defer dd.mu.Unlock()

	dd.seenComponents[path] = struct{}{}
}

// isSeen checks if a component path has been seen.
func (dd *DependencyDiscovery) isSeen(path string) bool {
	dd.mu.RLock()
	defer dd.mu.RUnlock()

	_, seen := dd.seenComponents[path]

	return seen
}

func (dd *DependencyDiscovery) discoverDependencies(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	dComponent component.Component,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return errors.New("max dependency depth reached while discovering dependencies")
	}

	// Stack configs don't have dependencies (at least for now),
	// so we can return early.
	if _, ok := dComponent.(*component.Stack); ok {
		return nil
	}

	unit, ok := dComponent.(*component.Unit)
	if !ok {
		return errors.New("expected Unit component but got different type")
	}

	cfg := unit.Config()
	if cfg == nil {
		err := Parse(dComponent, ctx, l, opts, dd.suppressParseErrors, dd.parserOptions)
		if err != nil {
			return errors.New(err)
		}

		cfg = unit.Config()
	}

	depPaths, err := extractDependencyPaths(cfg, dComponent)
	if err != nil {
		return errors.New(err)
	}

	if len(depPaths) == 0 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(dd.numWorkers)

	var (
		errs []error
		mu   sync.Mutex
	)

	for _, depPath := range depPaths {
		g.Go(func() error {
			depComponent, err := dd.dependencyToDiscover(dComponent, depPath)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return nil
			}

			if depComponent == nil {
				return nil
			}

			err = dd.discoverDependencies(ctx, l, opts, depComponent, depthRemaining-1)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()
			}

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

// dependencyToDiscover resolves a dependency path to a component that also needs to have its dependencies discovered.
//
// It handles checking if the component already exists from a prior phase of discovery, creating a new component if not,
// marking as external if it's outside the working directory of discovery, and linking dependencies.
// Returns nil if the dependency shouldn't be involved in discovery any further (e.g., already processed or ignored).
func (dd *DependencyDiscovery) dependencyToDiscover(
	dComponent component.Component,
	depPath string,
) (component.Component, error) {
	dDiscoveryCtx := dComponent.DiscoveryContext()
	if dDiscoveryCtx == nil {
		return nil, NewMissingDiscoveryContextError(dComponent.Path())
	}

	if dDiscoveryCtx.WorkingDir == "" {
		return nil, NewMissingWorkingDirectoryError(dComponent.Path())
	}

	copiedDiscoveryCtx := dDiscoveryCtx.Copy()

	// To be conservative, we're going to assume that users _never_ want to
	// destroy components discovered as a consequence of graph discovery on top of
	// Git discovery.
	//
	// e.g. --filter '...[HEAD^...HEAD]...' --filter-allow-destroy
	//
	// We're going to assume that the user's intent is to only destroy component(s) that were
	// discovered as being removed in HEAD relative to HEAD^, and that what they want is to
	// simply plan/apply the components discovered as a consequence of graph discovery
	// from the removed component(s).
	//
	// The dependency/dependents haven't been removed between the Git references, so what the user
	// probably wants is to simply plan/apply the components discovered as a consequence of graph discovery
	// from the removed component(s).
	if copiedDiscoveryCtx.Ref != "" {
		updatedArgs := slices.DeleteFunc(copiedDiscoveryCtx.Args, func(arg string) bool {
			return arg == "-destroy"
		})

		copiedDiscoveryCtx.Args = updatedArgs
	}

	if dd.isSeen(depPath) {
		c := dd.components.FindByPath(depPath)
		if c != nil {
			dComponent.AddDependency(c)
		}

		return nil, nil
	}

	c := dd.components.FindByPath(depPath)
	if c != nil {
		c.SetDiscoveryContext(copiedDiscoveryCtx)

		dd.markSeen(depPath)
		dComponent.AddDependency(c)

		return c, nil
	}

	// Create new component for further discovery
	//
	// TODO: This will need to change in the future to handle stacks.
	depComponent := component.NewUnit(depPath)

	// Always use the parent component's discovery context
	if isExternal(copiedDiscoveryCtx.WorkingDir, depPath) {
		depComponent.SetExternal()
	}

	depComponent.SetDiscoveryContext(copiedDiscoveryCtx)

	dComponent.AddDependency(depComponent)

	dependencyToDiscover, _ := dd.components.EnsureComponent(depComponent)

	dd.markSeen(depPath)

	return dependencyToDiscover, nil
}
