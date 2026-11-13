package discovery

import (
	"context"
	"errors"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// dependencyPaths is the dependencies one component declares, as the paths they resolve to.
type dependencyPaths struct {
	// err is why the paths could not be read from the component's config.
	err error
	// paths is the dependency paths, sorted and without duplicates, empty when err is set.
	paths []string
}

// pipelineRelations is the dependencies the pipeline read from the components it discovered, before any of them is
// linked to another component.
type pipelineRelations struct {
	discovery *Discovery
	opts      *options.TerragruntOptions
	store     *rewrite.Store
	// declared maps each component whose dependencies have been read to the dependencies it declares.
	declared map[component.Component]dependencyPaths
}

// readDependencyPaths parses c when nothing has yet and returns the dependencies it declares. A component that is
// not a unit declares none. The caller sets the parse phase on ctx.
func readDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	d *Discovery,
	c component.Component,
) dependencyPaths {
	unit, ok := c.(*component.Unit)
	if !ok {
		return dependencyPaths{}
	}

	if err := ensureParsed(ctx, l, v, s, c, opts, d); err != nil {
		return dependencyPaths{err: err}
	}

	paths, err := extractDependencyPaths(d.paths, unit.Config(), c)
	if err != nil {
		return dependencyPaths{err: err}
	}

	paths, err = stackDependencyPaths(ctx, l, v, opts, paths)
	if err != nil {
		return dependencyPaths{err: err}
	}

	return dependencyPaths{paths: paths}
}

// link adds the dependencies the pipeline read to components, which are the components discovery found. It first
// reads the dependencies of each unit among them that no worker read, such as one the graph phase reached. A
// dependency that is none of them becomes a unit that only its dependents reach, and link reads the dependencies of
// each such unit in turn, once per path, until none is new or the dependency depth runs out.
//
// Returns the errors of reading the dependencies, joined. Every component that could be read is linked either way.
func (r *pipelineRelations) link(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	components component.Components,
) error {
	d := r.discovery

	maxDepth := d.maxDependencyDepth
	if maxDepth <= 0 {
		maxDepth = defaultMaxDependencyDepth
	}

	ctx = contextWithParsePhase(ctx, parsePhaseTagRelationship)

	r.readUnread(ctx, l, v, components, nil)

	linker := &dependencyLinker{
		discovery:  d,
		discovered: make(map[string]component.Component, len(components)),
		reached:    map[string]*component.Unit{},
	}

	for _, c := range components {
		if _, ok := linker.discovered[c.Path()]; !ok {
			linker.discovered[c.Path()] = c
		}
	}

	var errs []error

	for _, c := range components {
		declared := r.declared[c]
		if declared.err != nil {
			errs = append(errs, declared.err)

			continue
		}

		linker.add(v, c, declared.paths)
	}

	baseDepth := parseDepthFromContext(ctx)

	for depth := 1; depth < maxDepth && len(linker.unread) > 0; depth++ {
		units := linker.unread
		linker.unread = nil

		for i, declared := range r.readAll(contextWithParseDepth(ctx, baseDepth+depth), l, v, units) {
			if declared.err != nil {
				errs = append(errs, declared.err)

				continue
			}

			linker.add(v, units[i], declared.paths)
		}
	}

	return errors.Join(errs...)
}

// linkForDependents links each of components to the dependencies it declares, so a traversal can find the dependents
// of any of them. A dependency that is none of them becomes a component the graph phase can reach. A component whose
// path is in unparsed is neither read nor linked.
//
// Returns the errors of reading the dependencies.
func (r *pipelineRelations) linkForDependents(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	components component.Components,
	unparsed map[string]struct{},
) []error {
	ctx = contextWithParsePhase(ctx, parsePhaseTagDependencyGraph)

	r.readUnread(ctx, l, v, components, unparsed)

	shared := component.NewThreadSafeComponents(r.discovery.paths, components)

	var errs []error

	for _, c := range components {
		if _, skip := unparsed[c.Path()]; skip {
			l.Debugf("Discovery: %s is outside every dependent boundary; not parsing it", c.Path())
			continue
		}

		declared := r.declared[c]
		if declared.err != nil {
			errs = append(errs, declared.err)

			continue
		}

		linkReached(v.FS, c, declared.paths, shared)
	}

	return errs
}

// linkReached links c to the component at each of depPaths, taking it from components or adding it there with the
// discovery context of a component the graph reached from c.
func linkReached(
	fsys vfs.FS,
	c component.Component,
	depPaths []string,
	components *component.ThreadSafeComponents,
) {
	parentCtx := c.DiscoveryContext()
	if parentCtx == nil {
		return
	}

	for _, depPath := range depPaths {
		dep := componentFromDependencyPath(fsys, depPath, components)

		assignGraphDiscoveryContext(fsys, dep, parentCtx, depPath)

		added, _ := components.EnsureComponent(dep)

		c.AddDependency(added)
	}
}

// adoptLinkedDependencies adds to components each component that one of them is linked to as a dependency, at any
// depth. The graph phase then links to those, where it would otherwise create a second component for the path of
// one that discovery did not find.
func adoptLinkedDependencies(components *component.ThreadSafeComponents) {
	pending := components.ToComponents()
	adopted := make(map[component.Component]struct{}, len(pending))

	for len(pending) > 0 {
		c := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if _, done := adopted[c]; done {
			continue
		}

		adopted[c] = struct{}{}

		for _, dep := range c.Dependencies() {
			components.EnsureComponent(dep)

			pending = append(pending, dep)
		}
	}
}

// traversedFrom names a component a dependency traversal continued from, and the boundary it stayed within.
type traversedFrom struct {
	c        component.Component
	boundary string
}

// discoverDependenciesFromRelations adds each of targets to the results of state, and with it the dependencies its
// graph expression traverses to, as [GraphPhase.discoverDependencies] does. It runs on one goroutine and reads the
// dependencies of the components at each depth together.
//
// A component that several traversals reach is traversed from again only by one with more depth left than any
// before it, so the results do not depend on the order of the traversals.
//
// Returns the errors of reading and resolving the dependencies.
func (p *GraphPhase) discoverDependenciesFromRelations(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	state *graphTraversalState,
	graphExprs []*filter.GraphExpressionInfo,
	targets []DiscoveryResult,
) []error {
	ctx = contextWithParsePhase(ctx, parsePhaseTagGraphDependencies)

	var errs []error

	depthLeft := map[traversedFrom]int{}

	exprs := make(map[int]*filter.GraphExpressionInfo, len(graphExprs))
	for _, graphExpr := range graphExprs {
		exprs[graphExpr.Index] = graphExpr
	}

	for _, target := range targets {
		graphExpr, ok := exprs[target.GraphExpressionIndex]
		if !ok {
			continue
		}

		c := target.Component

		state.addReached(c)

		if !graphExpr.Dependencies.Include {
			continue
		}

		boundary, err := dependencyBoundary(v.FS, state.discovery, c, graphExpr)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		traversal := &dependencyTraversal{phase: p, state: state, depthLeft: depthLeft, boundary: boundary}
		traversal.run(ctx, l, v, c, p.dependencyDepth(graphExpr))

		errs = append(errs, traversal.errs...)
	}

	return errs
}

// dependencyTraversal is the traversal of one graph expression from one of its targets to its dependencies.
type dependencyTraversal struct {
	// phase is the graph phase the traversal runs in, which holds the dependencies the pipeline read.
	phase *GraphPhase
	// state holds the results the traversal adds to.
	state *graphTraversalState
	// depthLeft holds the most depth any traversal of the phase had left at each component it continued from.
	depthLeft map[traversedFrom]int
	// boundary is the directory the traversal stays within, empty when it has none.
	boundary string
	// errs is the errors of reading and resolving the dependencies.
	errs []error
}

// run adds the dependencies of target to the results, depth dependencies deep.
func (t *dependencyTraversal) run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	target component.Component,
	depth int,
) {
	if !t.continuesFrom(target, depth) {
		return
	}

	baseDepth := parseDepthFromContext(ctx)
	reached := component.Components{target}

	for level := 0; level < depth && len(reached) > 0; level++ {
		t.phase.relations.readUnread(contextWithParseDepth(ctx, baseDepth+level), l, v, reached, nil)

		var next component.Components

		for _, c := range reached {
			next = append(next, t.dependenciesOf(l, v.FS, c, depth-level-1)...)
		}

		reached = next
	}
}

// dependenciesOf adds the dependencies of c to the results and returns the ones the traversal continues from with
// left more depth.
func (t *dependencyTraversal) dependenciesOf(
	l log.Logger,
	fsys vfs.FS,
	c component.Component,
	left int,
) component.Components {
	declared := t.phase.relations.declared[c]
	if declared.err != nil {
		t.errs = append(t.errs, declared.err)

		return nil
	}

	var next component.Components

	for _, depPath := range declared.paths {
		dep, ok := t.reach(l, fsys, c, depPath)
		if ok && t.continuesFrom(dep, left) {
			next = append(next, dep)
		}
	}

	return next
}

// reach links c to its dependency at depPath and adds the dependency to the results. It reports false for a
// dependency outside the boundary or one that cannot be resolved.
func (t *dependencyTraversal) reach(
	l log.Logger,
	fsys vfs.FS,
	c component.Component,
	depPath string,
) (component.Component, bool) {
	if t.boundary != "" && isExternal(fsys, t.boundary, depPath) {
		l.Debugf("Dependency %s is outside discovery boundary %s; skipping", depPath, t.boundary)

		return nil, false
	}

	dep, err := t.phase.resolveDependency(fsys, c, depPath, t.state.threadSafeComponents)
	if err != nil {
		t.errs = append(t.errs, err)

		return nil, false
	}

	t.state.addReached(dep)

	return dep, true
}

// continuesFrom reports whether the traversal goes on from c, which it reached with left more depth: it does when
// no traversal of the phase within the same boundary went on from c with as much. It records left when it does.
func (t *dependencyTraversal) continuesFrom(c component.Component, left int) bool {
	from := traversedFrom{c: c, boundary: t.boundary}
	if t.depthLeft[from] >= left {
		return false
	}

	t.depthLeft[from] = left

	return true
}

// readUnread reads the dependencies of each unit among components that has none in declared and whose path is not
// in unparsed, and records them in declared.
func (r *pipelineRelations) readUnread(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	components component.Components,
	unparsed map[string]struct{},
) {
	var unread []*component.Unit

	for _, c := range components {
		if _, read := r.declared[c]; read {
			continue
		}

		if _, skip := unparsed[c.Path()]; skip {
			continue
		}

		if unit, ok := c.(*component.Unit); ok {
			unread = append(unread, unit)
		}
	}

	for i, declared := range r.readAll(ctx, l, v, unread) {
		r.declared[unread[i]] = declared
	}
}

// readAll returns the dependencies each of units declares, in the order of units.
func (r *pipelineRelations) readAll(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	units []*component.Unit,
) []dependencyPaths {
	declared := make([]dependencyPaths, len(units))

	var g errgroup.Group

	g.SetLimit(max(r.discovery.numWorkers, defaultDiscoveryWorkers))

	for i, unit := range units {
		g.Go(func() error {
			declared[i] = readDependencyPaths(ctx, l, v, r.store, r.opts, r.discovery, unit)

			return nil
		})
	}

	// No task returns an error: each records its own in declared.
	if err := g.Wait(); err != nil {
		panic(err)
	}

	return declared
}

// dependencyLinker links components to their dependencies on one goroutine.
type dependencyLinker struct {
	discovery *Discovery
	// discovered maps the path of each component discovery found to it.
	discovered map[string]component.Component
	// reached maps the path of each dependency discovery did not find to the unit created for it.
	reached map[string]*component.Unit
	// unread is the units in reached whose dependencies have not been read, in the order they were reached.
	unread []*component.Unit
}

// add links c to the component at each of paths, creating a unit for a path that has none.
func (k *dependencyLinker) add(v *venv.Venv, c component.Component, paths []string) {
	for _, path := range paths {
		dep := k.dependency(v, path)

		if !slices.Contains(c.Dependencies(), dep) {
			c.AddDependency(dep)
		}
	}
}

// dependency returns the component at path: the one discovery found, or a unit that only its dependents reach.
func (k *dependencyLinker) dependency(v *venv.Venv, path string) component.Component {
	if dep, ok := k.discovered[path]; ok {
		return dep
	}

	if dep, ok := k.reached[path]; ok {
		return dep
	}

	dep := component.NewUnit(path)

	if k.discovery.discoveryContext != nil {
		discoveryCtx := k.discovery.discoveryContext.Copy()
		discoveryCtx.SuggestOrigin(component.OriginRelationshipDiscovery)
		dep.SetDiscoveryContext(discoveryCtx)

		if isExternal(v.FS, discoveryCtx.WorkingDir, path) {
			dep.SetExternal()
		}
	}

	k.reached[path] = dep
	k.unread = append(k.unread, dep)

	return dep
}
