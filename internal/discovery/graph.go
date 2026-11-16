package discovery

import (
	"context"
	"slices"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// graphTraversal traverses dependency/dependent relationships based on graph expressions.
type graphTraversal struct {
	// relations is the dependencies the pipeline read.
	relations *pipelineRelations
	// maxDepth is the maximum depth for dependency traversal.
	maxDepth int
}

// graphTraversalState consolidates shared state used across graph traversal functions.
type graphTraversalState struct {
	opts                 *options.TerragruntOptions
	discovery            *Discovery
	threadSafeComponents *component.ThreadSafeComponents
	seenComponents       *stringSet
	results              *foundResults
}

// newGraphTraversal returns the traversal of the graph filters of a discovery over relations, at most maxDepth
// deep where a filter names no depth. A maxDepth of zero or less means [defaultMaxDependencyDepth].
func newGraphTraversal(relations *pipelineRelations, maxDepth int) *graphTraversal {
	if maxDepth <= 0 {
		maxDepth = defaultMaxDependencyDepth
	}

	return &graphTraversal{relations: relations, maxDepth: maxDepth}
}

// run adds to the results the components the graph filters of d traverse to from their targets among candidates,
// and passes on the candidates that are neither a target nor a potential dependent. components are the ones
// already discovered.
//
// Returns the errors of reading and resolving dependencies, joined.
func (p *graphTraversal) run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	d *Discovery,
	components component.Components,
	candidates []DiscoveryResult,
) (*foundResults, error) {
	results := new(foundResults)

	allComponents := slices.Concat(components, resultsToComponents(candidates))

	var targets []DiscoveryResult

	for _, candidate := range candidates {
		switch candidate.Reason {
		case filter.CandidacyReasonGraphTarget:
			targets = append(targets, candidate)
		case filter.CandidacyReasonPotentialDependent:
			// Potential dependents are NOT passed through - they're only used
			// for building the dependency graph. If they're actual dependents,
			// they'll be discovered during dependent traversal.
		case filter.CandidacyReasonNone, filter.CandidacyReasonRequiresParse:
			results.candidates = append(results.candidates, candidate)
		}
	}

	state := &graphTraversalState{
		opts:                 opts,
		discovery:            d,
		threadSafeComponents: component.NewThreadSafeComponents(d.paths, allComponents),
		seenComponents:       newStringSet(),
		results:              results,
	}

	return results, errors.Join(p.traverseFromRelations(ctx, l, v, state, d.classifier.GraphExpressions(), targets)...)
}

// dependentWalkScope returns where the search for dependents of c above the working directory starts and the
// directory it stops at, which is empty when no boundary could be determined. The boundary is the explicit one of
// graphExpr when it has one, otherwise --discovery-boundary, otherwise the detected git root.
//
// Returns a [DiscoveryBoundaryScopeError] when the explicit boundary and the working directory are disjoint.
func dependentWalkScope(
	fsys vfs.FS,
	d *Discovery,
	c component.Component,
	graphExpr *filter.GraphExpressionInfo,
) (string, string, error) {
	startDir := d.workingDir
	boundaryRoot := d.dependentWalkBoundary()

	if graphExpr.Dependents.Boundary != "" {
		resolved, err := resolveGraphBoundary(fsys, d.workingDir, graphExpr.Dependents.Boundary)
		if err != nil {
			return "", "", err
		}

		if isExternal(fsys, resolved, startDir) && isExternal(fsys, startDir, resolved) {
			return "", "", NewDiscoveryBoundaryScopeError(resolved, startDir)
		}

		boundaryRoot = resolved
	}

	// Only fall back to the component's own working directory when the user
	// named no boundary at all; an explicit one must not be silently widened.
	if graphExpr.Dependents.Boundary == "" && d.discoveryBoundary == "" {
		if dCtx := c.DiscoveryContext(); dCtx != nil &&
			dCtx.WorkingDir != "" &&
			dCtx.WorkingDir != d.workingDir {
			startDir = dCtx.WorkingDir
			boundaryRoot = dCtx.WorkingDir
		}
	}

	// A boundary inside the working directory starts the walk at itself.
	if boundaryRoot != "" && isExternal(fsys, boundaryRoot, startDir) && !isExternal(fsys, startDir, boundaryRoot) {
		startDir = boundaryRoot
	}

	return startDir, boundaryRoot, nil
}

// addReached adds a component a graph expression selects or reaches to the results, once.
func (s *graphTraversalState) addReached(c component.Component) {
	if loaded := s.seenComponents.LoadOrStore(c.Path()); loaded {
		return
	}

	s.results.discovered = append(s.results.discovered, DiscoveryResult{
		Component: c,
		Status:    filter.StatusReadyForFilter,
		Reason:    filter.CandidacyReasonNone,
	})
}

// dependencyDepth returns how many dependencies deep graphExpr traverses from its target.
func (p *graphTraversal) dependencyDepth(graphExpr *filter.GraphExpressionInfo) int {
	if graphExpr.Dependencies.Depth > 0 {
		return graphExpr.Dependencies.Depth
	}

	return p.maxDepth
}

// dependentDepth returns how many dependents deep graphExpr traverses from its target.
func (p *graphTraversal) dependentDepth(graphExpr *filter.GraphExpressionInfo) int {
	if graphExpr.Dependents.Depth > 0 {
		return graphExpr.Dependents.Depth
	}

	return p.maxDepth
}

// dependencyBoundary returns the directory the dependency traversal of graphExpr from c stays within, empty when
// it has none. An inline "(dir)" operand overrides --discovery-boundary for the expression.
func dependencyBoundary(
	fsys vfs.FS,
	d *Discovery,
	c component.Component,
	graphExpr *filter.GraphExpressionInfo,
) (string, error) {
	if isWorktreeComponent(c) {
		return d.evaluationContext().TargetBoundary(graphExpr.Dependencies, c), nil
	}

	if graphExpr.Dependencies.Boundary == "" {
		return d.discoveryBoundary, nil
	}

	return resolveGraphBoundary(fsys, d.workingDir, graphExpr.Dependencies.Boundary)
}

// worktreeDependentRoot returns the directory of the Git worktree of c that the search for its dependents covers:
// the dependent boundary of graphExpr there, or the whole worktree without one. It is empty when the boundary is
// not a directory at the ref of c.
func worktreeDependentRoot(
	l log.Logger,
	fsys vfs.FS,
	d *Discovery,
	c component.Component,
	graphExpr *filter.GraphExpressionInfo,
) (string, error) {
	root := d.evaluationContext().TargetBoundary(graphExpr.Dependents, c)
	if root == "" {
		root = c.DiscoveryContext().WorkingDir
	}

	if _, ok, err := WorktreeWalkRoot(fsys, root, ""); err != nil || !ok {
		l.Debugf("Boundary %s is not a directory at %s; no dependents of %s there", root, c.DiscoveryContext().Ref, c.Path())
		return "", err
	}

	return root, nil
}

// resolveDependency resolves a dependency path to a component.
func (p *graphTraversal) resolveDependency(
	fsys vfs.FS,
	parent component.Component,
	depPath string,
	check config.AmbiguousConfigCheck,
	threadSafeComponents *component.ThreadSafeComponents,
) (component.Component, error) {
	parentCtx := parent.DiscoveryContext()
	if parentCtx == nil {
		return nil, NewMissingDiscoveryContextError(parent.Path())
	}

	if parentCtx.WorkingDir == "" {
		return nil, NewMissingWorkingDirectoryError(parent.Path())
	}

	depComponent, err := componentFromDependencyPath(fsys, depPath, check, threadSafeComponents)
	if err != nil {
		return nil, err
	}

	assignGraphDiscoveryContext(fsys, depComponent, parentCtx, depPath)

	addedComponent, _ := threadSafeComponents.EnsureComponent(depComponent)

	parent.AddDependency(addedComponent)

	return addedComponent, nil
}

// assignGraphDiscoveryContext gives a graph-discovered dependency its discovery
// context, derived from the parent that reached it.
//
// It is a no-op once the component already has a working directory, which keeps
// the assignment to the first parent to reach the component along any path.
func assignGraphDiscoveryContext(
	fsys vfs.FS,
	dep component.Component,
	parentCtx *component.DiscoveryContext,
	depPath string,
) {
	if dctx := dep.DiscoveryContext(); dctx != nil && dctx.WorkingDir != "" {
		return
	}

	dep.SetDiscoveryContext(graphDiscoveryContext(parentCtx))

	if isExternal(fsys, parentCtx.WorkingDir, depPath) {
		if ext, ok := dep.(*component.Unit); ok {
			ext.SetExternal()
		}
	}
}

// graphDiscoveryContext returns the discovery context for a unit that graph discovery
// reaches from the unit that owns ctx. The copy has no Git reference.
func graphDiscoveryContext(ctx *component.DiscoveryContext) *component.DiscoveryContext {
	copiedCtx := ctx.CopyWithNewOrigin(component.OriginGraphDiscovery)

	if ctx.RemovedInRef() {
		copiedCtx.Args = slices.DeleteFunc(copiedCtx.Args, iacargs.IsDestroyFlag)
	}

	copiedCtx.Ref = ""

	return copiedCtx
}

// isWorktreeComponent reports whether c was discovered in a Git worktree rather than the working tree.
func isWorktreeComponent(c component.Component) bool {
	dctx := c.DiscoveryContext()

	return dctx != nil && dctx.Ref != "" && dctx.WorkingDir != ""
}
