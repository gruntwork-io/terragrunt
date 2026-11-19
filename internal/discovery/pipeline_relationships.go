package discovery

import (
	"context"
	"errors"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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
	// discovery is the discovery the relations are read for.
	discovery *Discovery
	// opts is the options every parse runs with.
	opts *options.TerragruntOptions
	// store memoizes the parses.
	store *rewrite.Store
	// declared maps each component whose dependencies have been read to the dependencies it declares.
	declared map[component.Component]dependencyPaths
}

// readDependencyPaths parses c when nothing has yet and returns the dependencies it declares. A component that is
// not a unit declares none. The caller sets the parse phase on ctx.
//
// The paths are empty and err holds a [config.AmbiguousConfigError] when the check opts asks for is
// [config.RejectAmbiguousConfig] and a dependency directory holds both default unit config files.
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

	check := configbridge.AmbiguousConfigCheck(opts)

	for _, path := range paths {
		if err := config.RejectAmbiguousDefaultConfig(v.FS, path, check); err != nil {
			return dependencyPaths{err: err}
		}
	}

	return dependencyPaths{paths: paths}
}

// link adds the dependencies the pipeline read to components, which are the components discovery found. It first
// reads the dependencies of each unit among them that no worker read. A dependency that is none of them becomes a
// unit that only its dependents reach.
//
// link then walks from each discovered unit through the units only dependents reach, reading their dependencies as
// it goes, so that two discovered units joined only through such units are related. A walk ends once it has reached
// every other discovered unit, when it has nothing left to follow, or when the dependency depth runs out. A lone
// discovered unit has nothing to relate to, so link reads nothing past its direct dependencies.
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
		resolved := d.paths.Resolve(c.Path())
		if _, ok := linker.discovered[resolved]; !ok {
			linker.discovered[resolved] = c
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

	walks := newTransientWalks(linker, components)
	linked := map[component.Component]struct{}{}
	baseDepth := parseDepthFromContext(ctx)

	for depth := 1; depth < maxDepth && len(walks) > 0; depth++ {
		var units component.Components

		for _, w := range walks {
			for _, unit := range w.next {
				if _, ok := linked[unit]; ok {
					continue
				}

				linked[unit] = struct{}{}
				units = append(units, unit)
			}
		}

		r.readUnread(contextWithParseDepth(ctx, baseDepth+depth), l, v, units, nil)

		for _, unit := range units {
			declared := r.declared[unit]
			if declared.err != nil {
				errs = append(errs, declared.err)

				continue
			}

			linker.add(v, unit, declared.paths)
		}

		walks = slices.DeleteFunc(walks, func(w *transientWalk) bool {
			w.advance(linker)

			return w.done()
		})
	}

	return errors.Join(errs...)
}

// transientWalk is the walk from one discovered unit through the units that only dependents reach.
type transientWalk struct {
	// reached is the paths of the discovered units the walk has reached, starting with its own.
	reached map[string]struct{}
	// visited is the paths of the units the walk has reached that discovery did not find.
	visited map[string]struct{}
	// next is the units the walk reached last that discovery did not find, whose dependencies it follows next.
	next []*component.Unit
	// units is how many units discovery found.
	units int
}

// newTransientWalks returns a walk from each unit among components that still has a unit to follow and another
// discovered unit to reach. linker holds the first of components at each path.
func newTransientWalks(linker *dependencyLinker, components component.Components) []*transientWalk {
	units := 0

	for _, c := range linker.discovered {
		if _, ok := c.(*component.Unit); ok {
			units++
		}
	}

	var walks []*transientWalk

	for _, c := range components {
		if _, ok := c.(*component.Unit); !ok {
			continue
		}

		if first, _ := linker.discoveredAt(c.Path()); first != c {
			continue
		}

		w := &transientWalk{
			reached: map[string]struct{}{c.Path(): {}},
			visited: map[string]struct{}{},
			units:   units,
		}

		w.follow(linker, c.Dependencies())

		if !w.done() {
			walks = append(walks, w)
		}
	}

	return walks
}

// advance follows the dependencies of the units the walk reached last.
func (w *transientWalk) advance(linker *dependencyLinker) {
	following := w.next
	w.next = nil

	for _, unit := range following {
		w.follow(linker, unit.Dependencies())
	}
}

// follow records that the walk reached deps. A discovered unit among them counts as reached, the walk stops at a
// discovered stack, and it follows the dependencies of any other unit it has not visited next.
func (w *transientWalk) follow(linker *dependencyLinker, deps component.Components) {
	for _, dep := range deps {
		if found, ok := linker.discoveredAt(dep.Path()); ok {
			if _, isUnit := found.(*component.Unit); isUnit {
				w.reached[found.Path()] = struct{}{}
			}

			continue
		}

		unit, ok := dep.(*component.Unit)
		if !ok {
			continue
		}

		if _, ok := w.visited[unit.Path()]; ok {
			continue
		}

		w.visited[unit.Path()] = struct{}{}
		w.next = append(w.next, unit)
	}
}

// done reports whether the walk has reached every discovered unit or has nothing left to follow.
func (w *transientWalk) done() bool {
	return len(w.reached) >= w.units || len(w.next) == 0
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

		if err := linkGraphDependencies(
			v.FS,
			c,
			declared.paths,
			configbridge.AmbiguousConfigCheck(r.opts),
			shared,
		); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
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
	// discovery is the discovery the components are linked for.
	discovery *Discovery
	// discovered maps the path of each component discovery found, with symlinks resolved, to it.
	discovered map[string]component.Component
	// reached maps the path of each dependency discovery did not find to the unit created for it.
	reached map[string]*component.Unit
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
	if dep, ok := k.discoveredAt(path); ok {
		return dep
	}

	if dep, ok := k.reached[path]; ok {
		return dep
	}

	dep := component.NewUnit(path)
	dep.SetConfigFile(unitConfigFilename(v.FS, path))

	discoveryCtx := k.discovery.discoveryContext.Copy()
	discoveryCtx.SuggestOrigin(component.OriginRelationshipDiscovery)
	dep.SetDiscoveryContext(discoveryCtx)

	if isExternal(v.FS, discoveryCtx.WorkingDir, path) {
		dep.SetExternal()
	}

	k.reached[path] = dep

	return dep
}

// discoveredAt returns the component discovery found at path, comparing paths with symlinks resolved.
func (k *dependencyLinker) discoveredAt(path string) (component.Component, bool) {
	c, ok := k.discovered[k.discovery.paths.Resolve(path)]

	return c, ok
}
