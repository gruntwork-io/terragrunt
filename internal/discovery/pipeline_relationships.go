package discovery

import (
	"context"
	"errors"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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
	// declared maps each component a worker read to the dependencies it declares.
	declared map[component.Component]dependencyPaths
}

// readDependencyPaths parses c when nothing has yet and returns the dependencies it declares. A component that is
// not a unit declares none.
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

	ctx = contextWithParsePhase(ctx, parsePhaseTagRelationship)

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

	var unread []*component.Unit

	for _, c := range components {
		if _, read := r.declared[c]; read {
			continue
		}

		if unit, ok := c.(*component.Unit); ok {
			unread = append(unread, unit)
		}
	}

	for i, declared := range r.readAll(ctx, l, v, unread) {
		r.declared[unread[i]] = declared
	}

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
