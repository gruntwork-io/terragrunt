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
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"
)

// stringSet is a thread-safe set of strings using map and RWMutex.
// This is more performant than sync.Map for string keys with simple bool values.
type stringSet struct {
	m  map[string]struct{}
	mu sync.RWMutex
}

// newStringSet creates a new stringSet.
func newStringSet() *stringSet {
	return &stringSet{
		m: make(map[string]struct{}),
	}
}

// LoadOrStore returns true if the key was already present (loaded),
// false if the key was newly stored.
func (s *stringSet) LoadOrStore(key string) (loaded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.m[key]; ok {
		return true
	}

	s.m[key] = struct{}{}

	return false
}

// Load returns whether the key exists in the set.
func (s *stringSet) Load(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.m[key]

	return ok
}

// GraphPhase traverses dependency/dependent relationships based on graph expressions.
type GraphPhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
	// maxDepth is the maximum depth for dependency traversal.
	maxDepth int
}

// NewGraphPhase creates a new GraphPhase.
func NewGraphPhase(numWorkers, maxDepth int) *GraphPhase {
	if numWorkers <= 0 {
		numWorkers = defaultDiscoveryWorkers
	}

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
func (p *GraphPhase) Run(ctx context.Context, input *PhaseInput) PhaseOutput {
	discovered := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	candidates := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	errChan := make(chan error, p.numWorkers)
	done := make(chan struct{})

	go func() {
		defer close(discovered)
		defer close(candidates)
		defer close(errChan)
		defer close(done)

		p.runGraphDiscovery(ctx, input, discovered, candidates, errChan)
	}()

	return PhaseOutput{
		Discovered: discovered,
		Candidates: candidates,
		Done:       done,
		Errors:     errChan,
	}
}

// runGraphDiscovery performs the actual graph traversal.
func (p *GraphPhase) runGraphDiscovery(
	ctx context.Context,
	input *PhaseInput,
	discovered chan<- DiscoveryResult,
	candidates chan<- DiscoveryResult,
	errChan chan<- error,
) {
	discovery := input.Discovery
	if discovery == nil {
		return
	}

	classifier := input.Classifier
	if classifier == nil || !classifier.HasGraphFilters() {
		for _, candidate := range input.Candidates {
			if candidate.Reason != CandidacyReasonGraphTarget {
				select {
				case candidates <- candidate:
				case <-ctx.Done():
					return
				}
			}
		}

		return
	}

	graphExprs := classifier.GraphExpressions()
	if len(graphExprs) == 0 {
		return
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
		select {
		case candidates <- candidate:
		case <-ctx.Done():
			return
		}
	}

	seenComponents := newStringSet()

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
				err := p.processGraphTarget(
					ctx,
					input.Logger,
					input.Opts,
					discovery,
					candidate,
					graphExpr,
					threadSafeComponents,
					seenComponents,
					discovered,
				)
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
		select {
		case errChan <- err:
		default:
		}
	}

	if len(errs) > 0 {
		select {
		case errChan <- errors.Join(errs...):
		default:
		}
	}
}

// processGraphTarget processes a single graph expression target.
func (p *GraphPhase) processGraphTarget(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovery *Discovery,
	candidate DiscoveryResult,
	graphExpr *GraphExpressionInfo,
	threadSafeComponents *component.ThreadSafeComponents,
	seenComponents *stringSet,
	discovered chan<- DiscoveryResult,
) error {
	c := candidate.Component

	// Always add the target to discovered, regardless of ExcludeTarget.
	// The final filter evaluation will handle ExcludeTarget appropriately.
	// We need the target in the result set for the final evaluation to work
	// (it uses the target as the starting point for traversing dependents).
	if loaded := seenComponents.LoadOrStore(c.Path()); !loaded {
		select {
		case discovered <- DiscoveryResult{
			Component: c,
			Status:    StatusDiscovered,
			Reason:    CandidacyReasonNone,
			Phase:     PhaseGraph,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if graphExpr.IncludeDependencies {
		depth := p.maxDepth
		if graphExpr.DependencyDepth > 0 {
			depth = graphExpr.DependencyDepth
		}

		err := p.discoverDependencies(
			ctx, l, opts, discovery, c, threadSafeComponents,
			seenComponents, discovered, depth,
		)
		if err != nil {
			return err
		}
	}

	if graphExpr.IncludeDependents {
		depth := p.maxDepth
		if graphExpr.DependentDepth > 0 {
			depth = graphExpr.DependentDepth
		}

		err := p.discoverDependents(
			ctx, l, opts, discovery, c, threadSafeComponents,
			seenComponents, discovered, depth,
		)
		if err != nil {
			return err
		}

		if discovery.gitRoot != "" {
			// Use the discovery's workingDir as the starting point for dependent discovery.
			// This is important when the target was discovered from a worktree - dependents
			// exist in the original working directory, not in the worktree.
			startDir := discovery.workingDir
			l.Debugf("Starting upstream dependent discovery from %s to gitRoot %s", startDir, discovery.gitRoot)

			visitedDirs := newStringSet()

			err := p.discoverDependentsUpstream(
				ctx, l, opts, discovery, c, threadSafeComponents,
				seenComponents, visitedDirs, discovered, startDir, depth,
			)
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
	opts *options.TerragruntOptions,
	discovery *Discovery,
	c component.Component,
	threadSafeComponents *component.ThreadSafeComponents,
	seenComponents *stringSet,
	discovered chan<- DiscoveryResult,
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
		err := parseComponent(c, ctx, l, opts, discovery.suppressParseErrors, discovery.parserOptions)
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
				c, depPath, threadSafeComponents,
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

			if loaded := seenComponents.LoadOrStore(depComponent.Path()); !loaded {
				select {
				case discovered <- DiscoveryResult{
					Component: depComponent,
					Status:    StatusDiscovered,
					Reason:    CandidacyReasonNone,
					Phase:     PhaseGraph,
				}:
				case <-ctx.Done():
					return ctx.Err()
				}

				err = p.discoverDependencies(
					ctx, l, opts, discovery, depComponent,
					threadSafeComponents, seenComponents, discovered,
					depthRemaining-1,
				)
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
	opts *options.TerragruntOptions,
	discovery *Discovery,
	c component.Component,
	threadSafeComponents *component.ThreadSafeComponents,
	seenComponents *stringSet,
	discovered chan<- DiscoveryResult,
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
			if loaded := seenComponents.LoadOrStore(dependent.Path()); loaded {
				return nil
			}

			select {
			case discovered <- DiscoveryResult{
				Component: dependent,
				Status:    StatusDiscovered,
				Reason:    CandidacyReasonNone,
				Phase:     PhaseGraph,
			}:
			case <-ctx.Done():
				return ctx.Err()
			}

			err := p.discoverDependents(
				ctx, l, opts, discovery, dependent,
				threadSafeComponents, seenComponents, discovered,
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

// discoverDependentsUpstream discovers dependents by walking up the filesystem
// from the target component's directory to gitRoot (or filesystem root if gitRoot is empty).
// At each directory level, it walks down to find terragrunt configs and checks if they
// depend on the target component.
func (p *GraphPhase) discoverDependentsUpstream(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovery *Discovery,
	target component.Component,
	threadSafeComponents *component.ThreadSafeComponents,
	seenComponents *stringSet,
	visitedDirs *stringSet,
	discovered chan<- DiscoveryResult,
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

	gitRoot := discovery.gitRoot
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
	resolvedDiscoveryWorkingDir := util.ResolvePath(discovery.workingDir)

	var (
		errs  []error
		errMu sync.Mutex
	)

	g, walkCtx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	candidates := make(chan component.Component, p.numWorkers*channelBufferMultiplier)
	discoveredDependents := make(chan component.Component, p.numWorkers*channelBufferMultiplier)

	g.Go(func() error {
		defer close(candidates)

		walkFn := filepath.WalkDir
		if opts != nil && opts.Experiments.Evaluate(experiment.Symlinks) {
			walkFn = util.WalkDirWithSymlinks
		}

		err := walkFn(currentDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			default:
			}

			if d.IsDir() {
				if loaded := visitedDirs.LoadOrStore(path); loaded {
					return filepath.SkipDir
				}

				if err := skipDirIfIgnorable(path); err != nil {
					return err
				}

				return nil
			}

			base := filepath.Base(path)
			if !slices.Contains(discovery.configFilenames, base) {
				return nil
			}

			candidate := createComponentFromPath(path, discovery.configFilenames, discovery.discoveryContext)
			if candidate == nil {
				return nil
			}

			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			case candidates <- candidate:
			}

			return nil
		})
		if err != nil {
			errMu.Lock()

			errs = append(errs, err)

			errMu.Unlock()
		}

		return nil
	})

	checkedForTarget := newStringSet()

	g.Go(func() error {
		defer close(discoveredDependents)

		for candidate := range candidates {
			if loaded := checkedForTarget.LoadOrStore(candidate.Path()); loaded {
				continue
			}

			if seenComponents.Load(candidate.Path()) {
				continue
			}

			if _, ok := candidate.(*component.Stack); ok {
				continue
			}

			if candidate.Path() == target.Path() {
				continue
			}

			unit, ok := candidate.(*component.Unit)
			if !ok {
				continue
			}

			cfg := unit.Config()
			if cfg == nil {
				err := parseComponent(candidate, walkCtx, l, opts, discovery.suppressParseErrors, discovery.parserOptions)
				if err != nil {
					if !discovery.suppressParseErrors {
						errMu.Lock()

						errs = append(errs, err)

						errMu.Unlock()
					}

					continue
				}

				cfg = unit.Config()
			}

			deps, err := extractDependencyPaths(cfg, candidate)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()

				continue
			}

			canonicalCandidate, created := threadSafeComponents.EnsureComponent(candidate)
			if created {
				dCtx := target.DiscoveryContext()
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
				depComponent := threadSafeComponents.FindByPath(dep)
				if depComponent == nil {
					depComponent = component.NewUnit(dep)
					depComponent, _ = threadSafeComponents.EnsureComponent(depComponent)
				}

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
				case resolvedDep == resolvedTargetPath:
					// Direct match - link to the existing depComponent
					canonicalCandidate.AddDependency(depComponent)

					dependsOnTarget = true
				case targetRelSuffix != "":
					// Compare relative suffixes when target is from a worktree.
					// Use resolved paths to handle symlinks consistently.
					depRelSuffix := strings.TrimPrefix(resolvedDep, resolvedDiscoveryWorkingDir)
					if depRelSuffix == targetRelSuffix {
						// The dependency path matches the target's relative suffix.
						// Link to the actual target component instead of the path-based component,
						// so that the dependent relationship is properly established.
						canonicalCandidate.AddDependency(target)

						dependsOnTarget = true
					} else {
						canonicalCandidate.AddDependency(depComponent)
					}
				default:
					canonicalCandidate.AddDependency(depComponent)
				}
			}

			if dependsOnTarget {
				select {
				case <-walkCtx.Done():
					return walkCtx.Err()
				case discoveredDependents <- canonicalCandidate:
				}
			}
		}

		return nil
	})

	g.Go(func() error {
		for dependent := range discoveredDependents {
			if loaded := seenComponents.LoadOrStore(dependent.Path()); loaded {
				continue
			}

			l.Debugf("Found dependent during upstream walk: %s (depends on target), sending to discovered channel", dependent.Path())

			select {
			case discovered <- DiscoveryResult{
				Component: dependent,
				Status:    StatusDiscovered,
				Reason:    CandidacyReasonNone,
				Phase:     PhaseGraph,
			}:
				l.Debugf("Successfully sent %s to discovered channel", dependent.Path())
			case <-walkCtx.Done():
				l.Debugf("Context cancelled while sending %s to discovered channel", dependent.Path())
				return walkCtx.Err()
			}

			freshVisitedDirs := newStringSet()

			l.Debugf("Recursively discovering dependents of %s from %s", dependent.Path(), filepath.Dir(dependent.Path()))

			err := p.discoverDependentsUpstream(
				walkCtx, l, opts, discovery, dependent,
				threadSafeComponents, seenComponents, freshVisitedDirs, discovered,
				filepath.Dir(dependent.Path()), depthRemaining-1,
			)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()
			}
		}

		return nil
	})

	parentDir := filepath.Dir(currentDir)
	if parentDir != currentDir && depthRemaining > 0 {
		g.Go(func() error {
			err := p.discoverDependentsUpstream(
				walkCtx, l, opts, discovery, target,
				threadSafeComponents, seenComponents, visitedDirs, discovered,
				parentDir, depthRemaining-1,
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

	existing := threadSafeComponents.FindByPath(depPath)
	if existing != nil {
		parent.AddDependency(existing)
		return existing, nil
	}

	depComponent := component.NewUnit(depPath)

	if isExternal(parentCtx.WorkingDir, depPath) {
		depComponent.SetExternal()
	}

	parent.AddDependency(depComponent)

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

	return addedComponent, nil
}

// extractDependencyPaths extracts all dependency paths from a Terragrunt configuration.
func extractDependencyPaths(cfg *config.TerragruntConfig, c component.Component) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}

	maxDedupLen := len(cfg.TerragruntDependencies)
	if cfg.Dependencies != nil {
		maxDedupLen += len(cfg.Dependencies.Paths)
	}

	deduped := make(map[string]struct{}, maxDedupLen)

	errs := make([]error, 0, maxDedupLen)

	for _, dependency := range cfg.TerragruntDependencies {
		if dependency.Enabled != nil && !*dependency.Enabled {
			continue
		}

		if dependency.ConfigPath.Type() != cty.String {
			errs = append(errs, errors.New("dependency config path is not a string"))
			continue
		}

		depPath := dependency.ConfigPath.AsString()
		if !filepath.IsAbs(depPath) {
			depPath = filepath.Clean(filepath.Join(c.Path(), depPath))
		}

		depPath = util.ResolvePath(depPath)
		deduped[depPath] = struct{}{}
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Clean(filepath.Join(c.Path(), dependency))
			}

			dependency = util.ResolvePath(dependency)
			deduped[dependency] = struct{}{}
		}
	}

	depPaths := make([]string, 0, len(deduped))
	for depPath := range deduped {
		depPaths = append(depPaths, depPath)
	}

	if len(errs) > 0 {
		return depPaths, errors.Join(errs...)
	}

	return depPaths, nil
}
