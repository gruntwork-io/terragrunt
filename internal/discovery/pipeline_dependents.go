package discovery

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// dependentWalk names the directories one search for dependents covers: start and each directory above it, up to
// boundary.
type dependentWalk struct {
	start    string
	boundary string
}

// boundaryUnit is a unit found below a dependent boundary.
type boundaryUnit struct {
	// found is the unit as the walk found it.
	found *component.Unit
	// parsed is the unit whose config is read for its dependencies: the one discovery already holds at the path of
	// found when that reads the same config file, and found otherwise.
	parsed *component.Unit
}

// linksDependentsToBoundary reports whether the dependents of the graph target c come from
// [GraphPhase.linkDependentsToBoundaries] in place of [GraphPhase.discoverDependentsUpstream]. They do when the
// pipeline read the dependencies and c was found in the working directory of the discovery.
func (p *GraphPhase) linksDependentsToBoundary(state *graphTraversalState, c component.Component) bool {
	if p.relations == nil || isWorktreeComponent(c) {
		return false
	}

	dctx := c.DiscoveryContext()

	return dctx == nil || dctx.WorkingDir == "" || dctx.WorkingDir == state.discovery.workingDir
}

// linkDependentsToBoundaries links every unit up to the dependent boundary of each of targets to its dependencies,
// so the traversal of dependents reaches the ones outside the working directory. It walks the directories of each
// boundary once, however many targets share it, and reads the dependencies of the units there together.
//
// Returns the errors of resolving the boundaries, walking to them and reading the dependencies.
func (p *GraphPhase) linkDependentsToBoundaries(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	state *graphTraversalState,
	graphExprs []*filter.GraphExpressionInfo,
	targets []DiscoveryResult,
) []error {
	ctx = contextWithParsePhase(ctx, parsePhaseTagGraphDependents)

	exprs := make(map[int]*filter.GraphExpressionInfo, len(graphExprs))
	for _, graphExpr := range graphExprs {
		exprs[graphExpr.Index] = graphExpr
	}

	var errs []error

	walked := map[dependentWalk]struct{}{}

	for _, target := range targets {
		graphExpr, ok := exprs[target.GraphExpressionIndex]
		if !ok || !graphExpr.Dependents.Include || !p.linksDependentsToBoundary(state, target.Component) {
			continue
		}

		start, boundary, err := dependentWalkScope(v.FS, state.discovery, target.Component, graphExpr)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		walk := dependentWalk{start: start, boundary: boundary}
		if _, done := walked[walk]; done || boundary == "" {
			continue
		}

		walked[walk] = struct{}{}

		l.Debugf("Linking dependents from %s to boundary %s", start, boundary)

		errs = append(errs, p.linkUnitsOf(ctx, l, v, state, walk, target.Component)...)
	}

	return errs
}

// linkUnitsOf links each unit walk finds to its dependencies. A unit discovery did not hold yet takes its discovery
// context from target, and joins the components of state only once its dependencies have been read.
func (p *GraphPhase) linkUnitsOf(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	state *graphTraversalState,
	walk dependentWalk,
	target component.Component,
) []error {
	units, err := walk.units(ctx, v, state)
	if err != nil {
		return []error{err}
	}

	parsed := make(component.Components, 0, len(units))
	for _, unit := range units {
		parsed = append(parsed, unit.parsed)
	}

	p.relations.readUnread(ctx, l, v, parsed, nil)

	var errs []error

	for _, unit := range units {
		declared := p.relations.declared[unit.parsed]
		if declared.err != nil {
			errs = append(errs, declared.err)

			continue
		}

		if dctx := target.DiscoveryContext(); dctx != nil {
			unit.found.SetDiscoveryContext(graphDiscoveryContext(dctx))
		}

		linked, _ := state.threadSafeComponents.EnsureComponent(unit.found)

		linkGraphDependencies(v.FS, linked, declared.paths, state.threadSafeComponents)
	}

	return errs
}

// units returns the units below the start of the walk and below each directory above it, up to the boundary. A
// directory is walked once, and a start outside the boundary has no units.
func (w dependentWalk) units(
	ctx context.Context,
	v *venv.Venv,
	state *graphTraversalState,
) ([]boundaryUnit, error) {
	var units []boundaryUnit

	d := state.discovery
	walkDir := walkDirFunc(v, state.opts)
	visited := map[string]struct{}{}

	visit := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.IsDir() {
			if _, seen := visited[path]; seen {
				return filepath.SkipDir
			}

			visited[path] = struct{}{}

			return util.SkipDirIfIgnorable(entry.Name())
		}

		if !slices.Contains(d.configFilenames, filepath.Base(path)) {
			return nil
		}

		found, ok := createComponentFromPath(path, d.configFilenames, d.discoveryContext).(*component.Unit)
		if !ok {
			return nil
		}

		units = append(units, boundaryUnit{
			found:  found,
			parsed: upstreamParseUnit(state.threadSafeComponents.FindByPath(found.Path()), found),
		})

		return nil
	}

	for dir := w.start; dir != filepath.Dir(dir) && w.covers(dir); dir = filepath.Dir(dir) {
		if err := walkDir(dir, visit); err != nil {
			return nil, err
		}
	}

	return units, nil
}

// covers reports whether dir is the boundary of the walk or below it.
func (w dependentWalk) covers(dir string) bool {
	return dir == w.boundary || strings.HasPrefix(dir, w.boundary)
}
