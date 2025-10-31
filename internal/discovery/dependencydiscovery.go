package discovery

import (
	"context"
	"runtime"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// DependencyDiscovery is the configuration for a DependencyDiscovery.
type DependencyDiscovery struct {
	discoveryContext     *component.DiscoveryContext
	components           *component.ThreadSafeComponents
	externalDependencies *component.ThreadSafeComponents
	mu                   *sync.RWMutex
	seenComponents       map[string]struct{}
	workingDir           string
	parserOptions        []hclparse.Option
	maxDepth             int
	numWorkers           int
	discoverExternal     bool
	suppressParseErrors  bool
}

func NewDependencyDiscovery(components *component.ThreadSafeComponents) *DependencyDiscovery {
	return &DependencyDiscovery{
		components:           components,
		externalDependencies: component.NewThreadSafeComponents(component.Components{}),
		mu:                   &sync.RWMutex{},
		seenComponents:       make(map[string]struct{}),
		numWorkers:           runtime.NumCPU(),
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

// WithDiscoverExternalDependencies sets the discoverExternal flag to true,
// which determines whether to discover and include external dependencies in the final results.
func (dd *DependencyDiscovery) WithDiscoverExternalDependencies() *DependencyDiscovery {
	dd.discoverExternal = true

	return dd
}

// WithParserOptions sets custom HCL parser options for dependency discovery.
func (dd *DependencyDiscovery) WithParserOptions(options []hclparse.Option) *DependencyDiscovery {
	dd.parserOptions = options
	return dd
}

func (dd *DependencyDiscovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *DependencyDiscovery {
	dd.discoveryContext = discoveryContext

	return dd
}

// WithWorkingDir sets the working directory for determining if dependencies are external.
func (dd *DependencyDiscovery) WithWorkingDir(workingDir string) *DependencyDiscovery {
	dd.workingDir = workingDir
	return dd
}

func (dd *DependencyDiscovery) DiscoverAllDependencies(
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
			err := dd.DiscoverDependencies(ctx, l, opts, c, dd.maxDepth)
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

func (dd *DependencyDiscovery) DiscoverDependencies(
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

	if unit.Config() == nil {
		err := Parse(dComponent, ctx, l, opts, dd.suppressParseErrors, dd.parserOptions)
		if err != nil {
			return errors.New(err)
		}
	}

	cfg := unit.Config()

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
			depComponent := dd.dependencyToDiscover(dComponent, depPath)
			if depComponent == nil {
				return nil
			}

			err := dd.DiscoverDependencies(ctx, l, opts, depComponent, depthRemaining-1)
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
) component.Component {
	if dd.isSeen(depPath) {
		c := dd.components.FindByPath(depPath)
		if c != nil {
			dComponent.AddDependency(c)
		}

		return nil
	}

	c := dd.components.FindByPath(depPath)
	if c != nil {
		dd.markSeen(depPath)
		dComponent.AddDependency(c)

		return c
	}

	isExternal := isExternal(dd.workingDir, depPath)

	// If the dependency is external and discovery is disabled, we add the dependency to our external dependencies
	// set, ensure that we link it to the correct component, and mark it as seen.
	if isExternal && !dd.discoverExternal {
		existingDep := dd.externalDependencies.FindByPath(depPath)
		if existingDep != nil {
			dComponent.AddDependency(existingDep)
			dd.markSeen(depPath)

			return nil
		}

		depComponent := component.NewUnit(depPath)
		depComponent.SetExternal()

		if dd.discoveryContext != nil {
			depComponent.SetDiscoveryContext(dd.discoveryContext)
		}

		existingDep, _ = dd.externalDependencies.EnsureComponent(depComponent)
		dComponent.AddDependency(existingDep)

		dd.markSeen(depPath)

		return nil
	}

	// Create new component for further discovery
	//
	// TODO: This will need to change in the future to handle stacks.
	depComponent := component.NewUnit(depPath)

	if isExternal {
		depComponent.SetExternal()
	}

	if dd.discoveryContext != nil {
		depComponent.SetDiscoveryContext(dd.discoveryContext)
	}

	dComponent.AddDependency(depComponent)

	dependencyToDiscover, _ := dd.components.EnsureComponent(depComponent)

	dd.markSeen(depPath)

	return dependencyToDiscover
}

// FindComponentByPath searches for a component by path in both main components
// and external dependencies. Returns the component if found, nil otherwise.
func (dd *DependencyDiscovery) FindComponentByPath(path string) component.Component {
	c := dd.components.FindByPath(path)
	if c != nil {
		return c
	}

	return dd.externalDependencies.FindByPath(path)
}

// ExternalDependencies returns the external dependencies discovered during dependency discovery.
func (dd *DependencyDiscovery) ExternalDependencies() component.Components {
	return dd.externalDependencies.ToComponents()
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
