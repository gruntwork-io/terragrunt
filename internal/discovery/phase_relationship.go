package discovery

import (
	"context"
	"slices"
	"sync"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
	componentsByPath         map[string]component.Component
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
func (p *RelationshipPhase) Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
) (*PhaseResults, error) {
	results := NewPhaseResults()

	err := p.runRelationshipDiscovery(ctx, l, v, input, results)

	return results, err
}

// runRelationshipDiscovery performs the actual relationship discovery.
func (p *RelationshipPhase) runRelationshipDiscovery(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
	_ *PhaseResults,
) error {
	discovery := input.Discovery
	if discovery == nil || !discovery.discoverRelationships {
		return nil
	}

	interTransientComponents := component.NewThreadSafeComponents(v.FS, component.Components{})

	componentsByPath := make(map[string]component.Component, len(input.Components))

	for _, c := range input.Components {
		if _, ok := componentsByPath[c.Path()]; !ok {
			componentsByPath[c.Path()] = c
		}
	}

	state := &relationshipTraversalState{
		opts:                     input.Opts,
		discovery:                discovery,
		componentsByPath:         componentsByPath,
		interTransientComponents: interTransientComponents,
	}

	var (
		errs  = make([]error, 0, len(input.Components))
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, c := range input.Components {
		g.Go(func() error {
			tracker := newTerminalTracker(componentsByPath, c.Path())

			err := p.discoverRelationships(ctx, l, v, state, c, tracker, p.maxDepth)
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
	v *venv.Venv,
	state *relationshipTraversalState,
	c component.Component,
	tracker *terminalTracker,
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

	ctx = contextWithParsePhase(ctx, parsePhaseTagRelationship)

	if err := ensureParsed(ctx, l, v, c, state.opts, state.discovery); err != nil {
		return err
	}

	cfg := unit.Config()

	paths, err := extractDependencyPaths(v.FS, cfg, c)
	if err != nil {
		return err
	}

	paths, err = stackDependencyPaths(ctx, l, v, state.opts, paths)
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		return nil
	}

	depsToDiscover := make(component.Components, 0, len(paths))

	for _, path := range paths {
		// The boundary is deliberately not applied here: a dependency outside it
		// is still read and linked, so the units that do run can order against
		// it and fetch its outputs. [Discovery.dropOutsideBoundary] decides what
		// is returned.
		dep, created := state.dependencyToDiscover(v.FS, c, path)

		tracker.remove(dep.Path())

		if created {
			depsToDiscover = append(depsToDiscover, dep)
		}
	}

	if len(depsToDiscover) == 0 {
		return nil
	}

	if tracker.isEmpty() {
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
			err := p.discoverRelationships(
				contextWithIncrementedParseDepth(ctx),
				l,
				v,
				state,
				dep,
				tracker,
				depthRemaining-1,
			)
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
func (s *relationshipTraversalState) dependencyToDiscover(
	fsys vfs.FS,
	c component.Component,
	path string,
) (component.Component, bool) {
	if dep, ok := s.componentsByPath[path]; ok {
		if !slices.Contains(c.Dependencies(), dep) {
			c.AddDependency(dep)
		}

		return dep, false
	}

	newUnit := component.NewUnit(path)

	dep, created := s.interTransientComponents.EnsureComponent(fsys, newUnit)

	if created && s.discovery.discoveryContext != nil {
		discoveryCtx := s.discovery.discoveryContext.Copy()
		discoveryCtx.SuggestOrigin(component.OriginRelationshipDiscovery)
		dep.SetDiscoveryContext(discoveryCtx)

		if isExternal(fsys, discoveryCtx.WorkingDir, path) {
			dep.SetExternal()
		}
	}

	c.AddDependency(dep)

	return dep, created
}

// terminalTracker records which discovered components one traversal has
// reached as dependencies. Once it has reached all of them, relationship
// discovery can stop early. It is safe for concurrent use.
type terminalTracker struct {
	components map[string]component.Component
	seen       map[string]struct{}
	mu         sync.RWMutex
}

// newTerminalTracker returns a tracker over every component in components
// except the one at ownPath, which must be a key of components. The tracker
// never writes to components, so trackers can share one map.
func newTerminalTracker(components map[string]component.Component, ownPath string) *terminalTracker {
	return &terminalTracker{
		components: components,
		seen:       map[string]struct{}{ownPath: {}},
	}
}

func (t *terminalTracker) remove(path string) {
	if _, ok := t.components[path]; !ok {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.seen[path] = struct{}{}
}

func (t *terminalTracker) isEmpty() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.seen) == len(t.components)
}
