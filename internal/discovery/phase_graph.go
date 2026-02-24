package discovery

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"golang.org/x/sync/errgroup"
)

// GraphPhase traverses dependency/dependent relationships based on graph expressions.
type GraphPhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
	// maxDepth is the maximum depth for dependency traversal.
	maxDepth int
}

// graphTraversalState consolidates shared state used across graph traversal functions.
type graphTraversalState struct {
	opts                 *options.TerragruntOptions
	discovery            *Discovery
	threadSafeComponents *component.ThreadSafeComponents
	seenComponents       *stringSet
	results              *PhaseResults
}

// NewGraphPhase creates a new GraphPhase.
func NewGraphPhase(numWorkers, maxDepth int) *GraphPhase {
	numWorkers = max(numWorkers, defaultDiscoveryWorkers)

	if maxDepth <= 0 {
		maxDepth = defaultMaxDependencyDepth
	}

	return &GraphPhase{
		numWorkers: numWorkers,
		maxDepth:   maxDepth,
	}
}

// Name returns the human-readable name of the phase.
func (p *GraphPhase) Name() string {
	return "graph"
}

// Kind returns the PhaseKind identifier.
func (p *GraphPhase) Kind() PhaseKind {
	return PhaseGraph
}

// Run executes the graph discovery phase.
func (p *GraphPhase) Run(ctx context.Context, l log.Logger, input *PhaseInput) (*PhaseResults, error) {
	results := NewPhaseResults()

	discovery := input.Discovery
	if discovery == nil {
		return results, nil
	}

	classifier := input.Classifier
	if classifier == nil || !classifier.HasGraphFilters() {
		for _, candidate := range input.Candidates {
			if candidate.Reason != CandidacyReasonGraphTarget {
				results.AddCandidate(candidate)
			}
		}

		return results, nil
	}

	graphExprs := classifier.GraphExpressions()
	if len(graphExprs) == 0 {
		return results, nil
	}

	candidateComponents := resultsToComponents(input.Candidates)
	allComponents := make([]component.Component, 0, len(input.Components)+len(candidateComponents))
	allComponents = append(allComponents, input.Components...)
	allComponents = append(allComponents, candidateComponents...)
	threadSafeComponents := component.NewThreadSafeComponents(allComponents)

	graphTargetCandidates := make([]DiscoveryResult, 0, len(input.Candidates))
	otherCandidates := make([]DiscoveryResult, 0, len(input.Candidates))

	for _, candidate := range input.Candidates {
		switch candidate.Reason {
		case CandidacyReasonGraphTarget:
			graphTargetCandidates = append(graphTargetCandidates, candidate)
		case CandidacyReasonPotentialDependent:
			// Potential dependents are NOT passed through - they're only used
			// for building the dependency graph. If they're actual dependents,
			// they'll be discovered during dependent traversal.
		case CandidacyReasonNone, CandidacyReasonRequiresParse:
			otherCandidates = append(otherCandidates, candidate)
		}
	}

	for _, candidate := range otherCandidates {
		results.AddCandidate(candidate)
	}

	seenComponents := newStringSet()

	state := &graphTraversalState{
		opts:                 input.Opts,
		discovery:            discovery,
		threadSafeComponents: threadSafeComponents,
		seenComponents:       seenComponents,
		results:              results,
	}

	var (
		errs  []error
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, graphExpr := range graphExprs {
		matchingCandidates := make([]DiscoveryResult, 0, len(graphTargetCandidates))

		for _, candidate := range graphTargetCandidates {
			if candidate.GraphExpressionIndex == graphExpr.Index {
				matchingCandidates = append(matchingCandidates, candidate)
			}
		}

		if len(matchingCandidates) == 0 {
			continue
		}

		for _, candidate := range matchingCandidates {
			g.Go(func() error {
				err := p.processGraphTarget(ctx, l, state, candidate, graphExpr)
				if err != nil {
					errMu.Lock()

					errs = append(errs, err)

					errMu.Unlock()
				}

				return nil
			})
		}
	}

	if err := g.Wait(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return results, errors.Join(errs...)
	}

	return results, nil
}

// processGraphTarget processes a single graph expression target.
func (p *GraphPhase) processGraphTarget(
	ctx context.Context,
	l log.Logger,
	state *graphTraversalState,
	candidate DiscoveryResult,
	graphExpr *GraphExpressionInfo,
) error {
	c := candidate.Component

	// Always add the target to discovered, regardless of ExcludeTarget.
	// The final filter evaluation will handle ExcludeTarget appropriately.
	// We need the target in the result set for the final evaluation to work
	// (it uses the target as the starting point for traversing dependents).
	if loaded := state.seenComponents.LoadOrStore(c.Path()); !loaded {
		state.results.AddDiscovered(DiscoveryResult{
			Component: c,
			Status:    StatusDiscovered,
			Reason:    CandidacyReasonNone,
			Phase:     PhaseGraph,
		})
	}

	if graphExpr.IncludeDependencies {
		depth := p.maxDepth
		if graphExpr.DependencyDepth > 0 {
			depth = graphExpr.DependencyDepth
		}

		err := p.discoverDependencies(ctx, l, state, c, depth)
		if err != nil {
			return err
		}
	}

	if graphExpr.IncludeDependents {
		depth := p.maxDepth
		if graphExpr.DependentDepth > 0 {
			depth = graphExpr.DependentDepth
		}

		err := p.discoverDependents(ctx, l, state, c, depth)
		if err != nil {
			return err
		}

		if state.discovery.gitRoot != "" {
			// Use the discovery's workingDir as the starting point for dependent discovery.
			// This is important when the target was discovered from a worktree - dependents
			// exist in the original working directory, not in the worktree.
			startDir := state.discovery.workingDir
			l.Debugf(
				"Starting upstream dependent discovery from %s to gitRoot %s",
				startDir,
				state.discovery.gitRoot,
			)

			visitedDirs := newStringSet()

			err := p.discoverDependentsUpstream(ctx, l, state, c, visitedDirs, startDir, depth)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// discoverDependencies recursively discovers dependencies of a component.
func (p *GraphPhase) discoverDependencies(
	ctx context.Context,
	l log.Logger,
	state *graphTraversalState,
	c component.Component,
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

	depPaths, err := extractDependencyPaths(cfg, c)
	if err != nil {
		return err
	}

	if len(depPaths) == 0 {
		return nil
	}

	var (
		errs  []error
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, depPath := range depPaths {
		g.Go(func() error {
			depComponent, err := p.resolveDependency(
				c, depPath, state.threadSafeComponents,
			)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()

				return nil
			}

			if depComponent == nil {
				return nil
			}

			if loaded := state.seenComponents.LoadOrStore(depComponent.Path()); !loaded {
				state.results.AddDiscovered(DiscoveryResult{
					Component: depComponent,
					Status:    StatusDiscovered,
					Reason:    CandidacyReasonNone,
					Phase:     PhaseGraph,
				})

				err = p.discoverDependencies(ctx, l, state, depComponent, depthRemaining-1)
				if err != nil {
					errMu.Lock()

					errs = append(errs, err)

					errMu.Unlock()
				}
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

// discoverDependents discovers dependents of a component by traversing the existing graph.
func (p *GraphPhase) discoverDependents(
	ctx context.Context,
	l log.Logger,
	state *graphTraversalState,
	c component.Component,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return nil
	}

	dependents := c.Dependents()
	if len(dependents) == 0 {
		return nil
	}

	var (
		errs  []error
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, dependent := range dependents {
		g.Go(func() error {
			if loaded := state.seenComponents.LoadOrStore(dependent.Path()); loaded {
				return nil
			}

			state.results.AddDiscovered(DiscoveryResult{
				Component: dependent,
				Status:    StatusDiscovered,
				Reason:    CandidacyReasonNone,
				Phase:     PhaseGraph,
			})

			err := p.discoverDependents(ctx, l, state, dependent, depthRemaining-1)
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

// upstreamDiscoveryState holds shared state for processing upstream candidates.
// Created once per discoverDependentsUpstream call and reused across candidates.
type upstreamDiscoveryState struct {
	graphTraversalState         *graphTraversalState
	target                      component.Component
	checkedForTarget            *stringSet
	errs                        *[]error
	errMu                       *sync.Mutex
	resolvedTargetPath          string
	targetRelSuffix             string
	resolvedDiscoveryWorkingDir string
}

// discoverDependentsUpstream discovers dependents by walking up the filesystem
// from the target component's directory to gitRoot (or filesystem root if gitRoot is empty).
// At each directory level, it walks down to find terragrunt configs and checks if they
// depend on the target component.
func (p *GraphPhase) discoverDependentsUpstream(
	ctx context.Context,
	l log.Logger,
	state *graphTraversalState,
	target component.Component,
	visitedDirs *stringSet,
	currentDir string,
	depthRemaining int,
) error {
	l.Debugf("discoverDependentsUpstream: target=%s currentDir=%s depth=%d", target.Path(), currentDir, depthRemaining)

	if depthRemaining <= 0 {
		l.Debugf("discoverDependentsUpstream: depth limit reached")
		return nil
	}

	if currentDir == filepath.Dir(currentDir) {
		l.Debugf("discoverDependentsUpstream: reached filesystem root")
		return nil
	}

	gitRoot := state.discovery.gitRoot
	if gitRoot != "" && currentDir != gitRoot && !strings.HasPrefix(currentDir, gitRoot) {
		l.Debugf("discoverDependentsUpstream: outside git root boundary (currentDir=%s, gitRoot=%s)", currentDir, gitRoot)
		return nil
	}

	resolvedTargetPath := util.ResolvePath(target.Path())

	// When the target is from a worktree, we need to compare using relative suffixes
	// because the absolute paths will differ (worktree vs original directory).
	// We resolve paths to handle symlinks (e.g., /var -> /private/var on macOS).
	targetRelSuffix := ""

	if targetDCtx := target.DiscoveryContext(); targetDCtx != nil && targetDCtx.WorkingDir != "" {
		resolvedWorkingDir := util.ResolvePath(targetDCtx.WorkingDir)
		targetRelSuffix = strings.TrimPrefix(resolvedTargetPath, resolvedWorkingDir)
	}

	// Resolve discovery.workingDir for consistent path comparison.
	resolvedDiscoveryWorkingDir := util.ResolvePath(state.discovery.workingDir)

	var candidates []component.Component

	walkFn := filepath.WalkDir
	if state.opts != nil && state.opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFn = util.WalkDirWithSymlinks
	}

	err := walkFn(currentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if loaded := visitedDirs.LoadOrStore(path); loaded {
				return filepath.SkipDir
			}

			if err := util.SkipDirIfIgnorable(d.Name()); err != nil {
				return err
			}

			return nil
		}

		base := filepath.Base(path)
		if !slices.Contains(state.discovery.configFilenames, base) {
			return nil
		}

		candidate := createComponentFromPath(path, state.discovery.configFilenames, state.discovery.discoveryContext)
		if candidate != nil {
			candidates = append(candidates, candidate)
		}

		return nil
	})
	if err != nil {
		return err
	}

	var (
		discoveredDependents []component.Component
		dependentsMu         sync.Mutex
		errs                 []error
		errMu                sync.Mutex
	)

	upstreamState := &upstreamDiscoveryState{
		graphTraversalState:         state,
		target:                      target,
		checkedForTarget:            newStringSet(),
		resolvedTargetPath:          resolvedTargetPath,
		targetRelSuffix:             targetRelSuffix,
		resolvedDiscoveryWorkingDir: resolvedDiscoveryWorkingDir,
		errs:                        &errs,
		errMu:                       &errMu,
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, candidate := range candidates {
		g.Go(func() error {
			dependent := p.processUpstreamCandidate(gCtx, l, upstreamState, candidate)
			if dependent != nil {
				dependentsMu.Lock()

				discoveredDependents = append(discoveredDependents, dependent)

				dependentsMu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for _, dependent := range discoveredDependents {
		if loaded := state.seenComponents.LoadOrStore(dependent.Path()); loaded {
			continue
		}

		l.Debugf("Found dependent during upstream walk: %s (depends on target), adding to results", dependent.Path())

		state.results.AddDiscovered(DiscoveryResult{
			Component: dependent,
			Status:    StatusDiscovered,
			Reason:    CandidacyReasonNone,
			Phase:     PhaseGraph,
		})

		l.Debugf("Successfully added %s to results", dependent.Path())

		freshVisitedDirs := newStringSet()

		l.Debugf("Recursively discovering dependents of %s from %s", dependent.Path(), filepath.Dir(dependent.Path()))

		err := p.discoverDependentsUpstream(
			ctx, l, state, dependent, freshVisitedDirs,
			filepath.Dir(dependent.Path()), depthRemaining-1,
		)
		if err != nil {
			errs = append(errs, err)
		}
	}

	parentDir := filepath.Dir(currentDir)
	if parentDir != currentDir && depthRemaining > 0 {
		err := p.discoverDependentsUpstream(
			ctx, l, state, target, visitedDirs,
			parentDir, depthRemaining-1,
		)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// processUpstreamCandidate processes a single candidate to check if it depends on the target.
// Returns the canonical component if it depends on the target, nil otherwise.
// This function is designed to be called concurrently from multiple goroutines.
func (p *GraphPhase) processUpstreamCandidate(
	ctx context.Context,
	l log.Logger,
	state *upstreamDiscoveryState,
	candidate component.Component,
) component.Component {
	if loaded := state.checkedForTarget.LoadOrStore(candidate.Path()); loaded {
		return nil
	}

	if state.graphTraversalState.seenComponents.Load(candidate.Path()) {
		return nil
	}

	if _, ok := candidate.(*component.Stack); ok {
		return nil
	}

	if candidate.Path() == state.target.Path() {
		return nil
	}

	unit, ok := candidate.(*component.Unit)
	if !ok {
		return nil
	}

	cfg := unit.Config()
	if cfg == nil {
		err := parseComponent(ctx, l, candidate, state.graphTraversalState.opts, state.graphTraversalState.discovery)
		if err != nil {
			if !state.graphTraversalState.discovery.suppressParseErrors {
				state.errMu.Lock()

				*state.errs = append(*state.errs, err)

				state.errMu.Unlock()
			}

			return nil
		}

		cfg = unit.Config()
	}

	deps, err := extractDependencyPaths(cfg, candidate)
	if err != nil {
		state.errMu.Lock()

		*state.errs = append(*state.errs, err)

		state.errMu.Unlock()

		return nil
	}

	canonicalCandidate, created := state.graphTraversalState.threadSafeComponents.EnsureComponent(candidate)
	if created {
		dCtx := state.target.DiscoveryContext()
		if dCtx != nil {
			copiedCtx := dCtx.CopyWithNewOrigin(component.OriginGraphDiscovery)

			// Clear the Ref and related args for graph-discovered components.
			// They shouldn't inherit the git ref from the target, as this would
			// cause them to match git filters and become targets themselves.
			copiedCtx.Ref = ""
			copiedCtx.Args = slices.DeleteFunc(copiedCtx.Args, func(arg string) bool {
				return arg == "-destroy"
			})

			canonicalCandidate.SetDiscoveryContext(copiedCtx)
		}
	}

	dependsOnTarget := false

	for _, dep := range deps {
		depComponent := componentFromDependencyPath(dep, state.graphTraversalState.threadSafeComponents)
		depComponent, _ = state.graphTraversalState.threadSafeComponents.EnsureComponent(depComponent)

		parentCtx := canonicalCandidate.DiscoveryContext()
		if parentCtx != nil && isExternal(parentCtx.WorkingDir, dep) {
			if ext, ok := depComponent.(*component.Unit); ok {
				ext.SetExternal()
			}
		}

		// Compare paths: first try exact match, then try relative suffix match
		// for worktree scenarios where target is in a different directory.
		resolvedDep := util.ResolvePath(dep)

		switch {
		case resolvedDep == state.resolvedTargetPath:
			// Direct match - link to the existing depComponent
			canonicalCandidate.AddDependency(depComponent)

			dependsOnTarget = true
		case state.targetRelSuffix != "":
			// Compare relative suffixes when target is from a worktree.
			// Use resolved paths to handle symlinks consistently.
			depRelSuffix := strings.TrimPrefix(resolvedDep, state.resolvedDiscoveryWorkingDir)
			if depRelSuffix == state.targetRelSuffix {
				// The dependency path matches the target's relative suffix.
				// Link to the actual target component instead of the path-based component,
				// so that the dependent relationship is properly established.
				canonicalCandidate.AddDependency(state.target)

				dependsOnTarget = true
			} else {
				canonicalCandidate.AddDependency(depComponent)
			}
		default:
			canonicalCandidate.AddDependency(depComponent)
		}
	}

	if dependsOnTarget {
		return canonicalCandidate
	}

	return nil
}

// resolveDependency resolves a dependency path to a component.
func (p *GraphPhase) resolveDependency(
	parent component.Component,
	depPath string,
	threadSafeComponents *component.ThreadSafeComponents,
) (component.Component, error) {
	parentCtx := parent.DiscoveryContext()
	if parentCtx == nil {
		return nil, NewMissingDiscoveryContextError(parent.Path())
	}

	if parentCtx.WorkingDir == "" {
		return nil, NewMissingWorkingDirectoryError(parent.Path())
	}

	depComponent := componentFromDependencyPath(depPath, threadSafeComponents)

	addedComponent, created := threadSafeComponents.EnsureComponent(depComponent)
	if created {
		copiedCtx := parentCtx.CopyWithNewOrigin(component.OriginGraphDiscovery)

		// Clear the Ref and related args for graph-discovered dependencies.
		// They shouldn't inherit the git ref from the parent, as this would
		// cause them to match git filters and become targets themselves.
		copiedCtx.Ref = ""
		copiedCtx.Args = slices.DeleteFunc(copiedCtx.Args, func(arg string) bool {
			return arg == "-destroy"
		})

		depComponent.SetDiscoveryContext(copiedCtx)
	}

	if isExternal(parentCtx.WorkingDir, depPath) {
		if ext, ok := addedComponent.(*component.Unit); ok {
			ext.SetExternal()
		}
	}

	parent.AddDependency(addedComponent)

	return addedComponent, nil
}
