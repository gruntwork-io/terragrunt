package discovery

import (
	"context"
	"runtime"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// RelationshipDiscovery is the configuration for a RelationshipDiscovery.
//
// It's used to discover any potential relationships between components.
type RelationshipDiscovery struct {
	components               *component.Components           // all components that have been discovered (not just the ones that require relationship discovery)
	interTransientComponents *component.ThreadSafeComponents // components that are discovered while trying to work out relationships between components, but are not officially discovered
	parserOptions            []hclparse.Option               // the parser options to use when parsing configurations
	maxDepth                 int                             // the maximum depth to discover relationships at
	numWorkers               int                             // the number of workers to use to discover relationships
}

// NewRelationshipDiscovery creates a new RelationshipDiscovery with the given configuration.
func NewRelationshipDiscovery(components *component.Components) *RelationshipDiscovery {
	return &RelationshipDiscovery{
		components:               components,
		interTransientComponents: component.NewThreadSafeComponents(component.Components{}),
		numWorkers:               runtime.NumCPU(),
	}
}

// WithMaxDepth sets the maximum depth for relationship discovery.
func (rd *RelationshipDiscovery) WithMaxDepth(maxDepth int) *RelationshipDiscovery {
	rd.maxDepth = maxDepth
	return rd
}

// WithNumWorkers sets the number of workers for relationship discovery.
func (rd *RelationshipDiscovery) WithNumWorkers(numWorkers int) *RelationshipDiscovery {
	rd.numWorkers = numWorkers
	return rd
}

// WithParserOptions sets the parser options for relationship discovery.
func (rd *RelationshipDiscovery) WithParserOptions(parserOptions []hclparse.Option) *RelationshipDiscovery {
	rd.parserOptions = parserOptions
	return rd
}

// Discover discovers any potential relationships between all components.
func (rd *RelationshipDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	orphanComponents component.Components,
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(rd.numWorkers)

	var (
		errs []error
		mu   sync.Mutex
	)

	for _, c := range orphanComponents {
		g.Go(func() error {
			// terminalComponents are the components that, if discovered, would indicate that the component
			// can terminate discovery.
			//
			// For that reason, the list is all components that are not the component undergoing relationship discovery.
			terminalComponents := slices.Collect(func(yield func(component.Component) bool) {
				for _, rc := range *rd.components {
					if rc != nil && rc != c {
						if !yield(rc) {
							return
						}
					}
				}
			})

			err := rd.discoverRelationships(ctx, l, opts, c, terminalComponents, rd.maxDepth)
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

// discoverRelationships discovers any potential relationships between a single component and all other components.
func (rd *RelationshipDiscovery) discoverRelationships(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	c component.Component,
	terminalComponents component.Components,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return errors.New("max depth reached while discovering relationships")
	}

	if _, ok := c.(*component.Stack); ok {
		return nil
	}

	unit, ok := c.(*component.Unit)
	if !ok {
		return errors.New("expected Unit component but got different type")
	}

	cfg := unit.Config()
	if cfg == nil {
		err := Parse(c, ctx, l, opts, true, rd.parserOptions)
		if err != nil {
			return errors.New(err)
		}

		cfg = unit.Config()
	}

	paths, err := extractDependencyPaths(cfg, c)
	if err != nil {
		return errors.New(err)
	}

	if len(paths) == 0 {
		return nil
	}

	depsToDiscover := make(component.Components, 0, len(paths))

	for _, path := range paths {
		dep, created := rd.dependencyToDiscover(c, path)

		// Delete the entry from terminal components if it's found.
		terminalComponents = slices.DeleteFunc(terminalComponents, func(tc component.Component) bool {
			return tc != nil && tc.Path() == dep.Path()
		})

		// We only want to recursively discover dependencies if we ended up creating a new component.
		//
		// If the component already existed, something else will be working on discovering its dependencies
		// (either it was in the initial list of components to discover relationships for, or it was discovered
		// as a dependency of another component).
		if created {
			depsToDiscover = append(depsToDiscover, dep)
		}
	}

	// If there are no dependencies to discover, there's nothing left to do,
	// and we can return early.
	if len(depsToDiscover) == 0 {
		return nil
	}

	// If we've successfully encountered all terminal components, we can return early.
	if len(terminalComponents) == 0 {
		return nil
	}

	var (
		errs []error
		mu   sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(rd.numWorkers)

	for _, dep := range depsToDiscover {
		g.Go(func() error {
			err := rd.discoverRelationships(ctx, l, opts, dep, terminalComponents, depthRemaining-1)
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

func (rd *RelationshipDiscovery) dependencyToDiscover(c component.Component, path string) (component.Component, bool) {
	for _, dep := range *rd.components {
		if dep.Path() == path {
			if !slices.Contains(c.Dependencies(), dep) {
				c.AddDependency(dep)
			}

			return dep, false
		}
	}

	// This will need to change in the future to handle stacks.

	newUnit := component.NewUnit(path)

	dep, created := rd.interTransientComponents.EnsureComponent(newUnit)

	c.AddDependency(dep)

	return dep, created
}
