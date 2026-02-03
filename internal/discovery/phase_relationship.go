package discovery

import (
	"context"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"golang.org/x/sync/errgroup"
)

// RelationshipPhase builds dependency relationships between discovered components.
// It discovers dependencies of "orphan" components (those without known dependencies)
// to build a complete dependency graph for execution ordering.
type RelationshipPhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
	// maxDepth is the maximum depth for relationship discovery.
	maxDepth int
}

// relationshipTraversalState consolidates state for relationship discovery.
type relationshipTraversalState struct {
	opts                     *options.TerragruntOptions
	discovery                *Discovery
	allComponents            *component.Components
	interTransientComponents *component.ThreadSafeComponents
}

// NewRelationshipPhase creates a new RelationshipPhase.
func NewRelationshipPhase(numWorkers, maxDepth int) *RelationshipPhase {
	numWorkers = max(numWorkers, defaultDiscoveryWorkers)

	if maxDepth <= 0 {
		maxDepth = defaultMaxDependencyDepth
	}

	return &RelationshipPhase{
		numWorkers: numWorkers,
		maxDepth:   maxDepth,
	}
}

// Name returns the human-readable name of the phase.
func (p *RelationshipPhase) Name() string {
	return "relationship"
}

// Kind returns the PhaseKind identifier.
func (p *RelationshipPhase) Kind() PhaseKind {
	return PhaseRelationship
}

// Run executes the relationship discovery phase.
func (p *RelationshipPhase) Run(ctx context.Context, l log.Logger, input *PhaseInput) (*PhaseResults, error) {
	results := NewPhaseResults()

	err := p.runRelationshipDiscovery(ctx, l, input, results)

	return results, err
}

// runRelationshipDiscovery performs the actual relationship discovery.
func (p *RelationshipPhase) runRelationshipDiscovery(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	_ *PhaseResults,
) error {
	discovery := input.Discovery
	if discovery == nil || !discovery.discoverRelationships {
		return nil
	}

	interTransientComponents := component.NewThreadSafeComponents(component.Components{})

	state := &relationshipTraversalState{
		opts:                     input.Opts,
		discovery:                discovery,
		allComponents:            &input.Components,
		interTransientComponents: interTransientComponents,
	}

	var (
		errs  = make([]error, 0, len(input.Components))
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, c := range input.Components {
		// terminalComponents are components that, if encountered, indicate we can stop
		// traversal, as they are terminal components in the dependency graph.
		terminalComponents := slices.Collect(func(yield func(component.Component) bool) {
			for _, rc := range input.Components {
				if rc != nil && rc != c {
					if !yield(rc) {
						return
					}
				}
			}
		})

		g.Go(func() error {
			err := p.discoverRelationships(ctx, l, state, c, terminalComponents, p.maxDepth)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// discoverRelationships discovers dependencies for a single component.
func (p *RelationshipPhase) discoverRelationships(
	ctx context.Context,
	l log.Logger,
	state *relationshipTraversalState,
	c component.Component,
	terminalComponents component.Components,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return nil
	}

	if _, ok := c.(*component.Stack); ok {
		return nil
	}

	unit, ok := c.(*component.Unit)
	if !ok {
		return nil
	}

	cfg := unit.Config()
	if cfg == nil {
		err := parseComponent(ctx, l, c, state.opts, state.discovery)
		if err != nil {
			return err
		}

		cfg = unit.Config()
	}

	paths, err := extractDependencyPaths(cfg, c)
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		return nil
	}

	depsToDiscover := make(component.Components, 0, len(paths))

	for _, path := range paths {
		dep, created := p.dependencyToDiscover(c, path, state.allComponents, state.interTransientComponents, state.discovery)

		terminalComponents = slices.DeleteFunc(terminalComponents, func(tc component.Component) bool {
			return tc != nil && tc.Path() == dep.Path()
		})

		if created {
			depsToDiscover = append(depsToDiscover, dep)
		}
	}

	if len(depsToDiscover) == 0 {
		return nil
	}

	if len(terminalComponents) == 0 {
		return nil
	}

	var (
		errs  = make([]error, 0, len(depsToDiscover))
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, dep := range depsToDiscover {
		g.Go(func() error {
			err := p.discoverRelationships(ctx, l, state, dep, terminalComponents, depthRemaining-1)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()
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

// dependencyToDiscover resolves a dependency path and links it to the component.
func (p *RelationshipPhase) dependencyToDiscover(
	c component.Component,
	path string,
	allComponents *component.Components,
	interTransientComponents *component.ThreadSafeComponents,
	discovery *Discovery,
) (component.Component, bool) {
	for _, dep := range *allComponents {
		if dep.Path() == path {
			if !slices.Contains(c.Dependencies(), dep) {
				c.AddDependency(dep)
			}

			return dep, false
		}
	}

	newUnit := component.NewUnit(path)

	dep, created := interTransientComponents.EnsureComponent(newUnit)

	if discovery.discoveryContext != nil {
		discoveryCtx := discovery.discoveryContext.Copy()
		discoveryCtx.SuggestOrigin(component.OriginRelationshipDiscovery)
		dep.SetDiscoveryContext(discoveryCtx)

		if isExternal(discoveryCtx.WorkingDir, path) {
			dep.SetExternal()
		}
	}

	c.AddDependency(dep)

	return dep, created
}
