package discovery

import (
	"context"
	"path/filepath"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"

	"golang.org/x/sync/errgroup"
)

// New creates a new Discovery instance with the given working directory.
func New(workingDir string) *Discovery {
	return &Discovery{
		workingDir:         workingDir,
		maxDependencyDepth: defaultMaxDependencyDepth,
		numWorkers:         defaultDiscoveryWorkers,
	}
}

// WithDiscoveryContext sets the discovery context.
func (d *Discovery) WithDiscoveryContext(ctx *component.DiscoveryContext) *Discovery {
	d.discoveryContext = ctx
	return d
}

// WithWorktrees sets the worktrees for Git-based filters.
func (d *Discovery) WithWorktrees(w *worktrees.Worktrees) *Discovery {
	d.worktrees = w
	return d
}

// WithReport sets the report for recording excluded external dependencies.
func (d *Discovery) WithReport(r *report.Report) *Discovery {
	d.report = r
	return d
}

// WithConfigFilenames sets the config filenames to discover.
func (d *Discovery) WithConfigFilenames(filenames []string) *Discovery {
	d.configFilenames = filenames
	return d
}

// WithParserOptions sets custom HCL parser options.
func (d *Discovery) WithParserOptions(opts []hclparse.Option) *Discovery {
	d.parserOptions = opts
	return d
}

// WithFilters sets filter queries for component selection.
func (d *Discovery) WithFilters(filters filter.Filters) *Discovery {
	d.filters = filters

	// If there are any positive filters, exclude by default
	if d.filters.HasPositiveFilter() {
		d.excludeByDefault = true
	}

	// Check if filters require parsing
	if _, ok := d.filters.RequiresParse(); ok {
		d.requiresParse = true
	}

	// Collect Git expressions
	d.gitExpressions = d.filters.UniqueGitFilters()

	return d
}

// WithMaxDependencyDepth sets the maximum dependency depth.
func (d *Discovery) WithMaxDependencyDepth(depth int) *Discovery {
	d.maxDependencyDepth = depth
	return d
}

// WithNumWorkers sets the number of concurrent workers.
func (d *Discovery) WithNumWorkers(numWorkers int) *Discovery {
	if numWorkers > 0 && numWorkers <= maxDiscoveryWorkers {
		d.numWorkers = numWorkers
	}

	return d
}

// WithNoHidden excludes hidden directories from discovery.
func (d *Discovery) WithNoHidden() *Discovery {
	d.noHidden = true
	return d
}

// WithRequiresParse enables parsing of Terragrunt configurations.
func (d *Discovery) WithRequiresParse() *Discovery {
	d.requiresParse = true
	return d
}

// WithParseExclude enables parsing of exclude configurations.
func (d *Discovery) WithParseExclude() *Discovery {
	d.parseExclude = true
	d.requiresParse = true

	return d
}

// WithParseIncludes enables parsing for include configurations.
func (d *Discovery) WithParseIncludes() *Discovery {
	d.parseIncludes = true
	d.requiresParse = true

	return d
}

// WithReadFiles enables parsing for file reading information.
func (d *Discovery) WithReadFiles() *Discovery {
	d.readFiles = true
	d.requiresParse = true

	return d
}

// WithSuppressParseErrors suppresses errors during parsing.
func (d *Discovery) WithSuppressParseErrors() *Discovery {
	d.suppressParseErrors = true
	return d
}

// WithBreakCycles enables breaking cycles in the dependency graph.
func (d *Discovery) WithBreakCycles() *Discovery {
	d.breakCycles = true
	return d
}

// WithRelationships enables relationship discovery.
func (d *Discovery) WithRelationships() *Discovery {
	d.discoverRelationships = true
	return d
}

// WithGitRoot sets the git root directory for dependent discovery boundary.
func (d *Discovery) WithGitRoot(gitRoot string) *Discovery {
	d.gitRoot = gitRoot
	return d
}

// WithGraphTarget sets the graph target so discovery can prune to the target and its dependents.
func (d *Discovery) WithGraphTarget(target string) *Discovery {
	d.graphTarget = target
	return d
}

// WithOptions ingests runner options and applies any discovery-relevant settings.
// Currently, it extracts HCL parser options provided via common.ParseOptionsProvider
// and graph target options, and forwards them to discovery's configuration.
func (d *Discovery) WithOptions(opts ...any) *Discovery {
	var parserOptions []hclparse.Option

	for _, opt := range opts {
		if p, ok := opt.(interface{ GetParseOptions() []hclparse.Option }); ok {
			parserOptions = append(parserOptions, p.GetParseOptions()...)
		}

		if g, ok := opt.(interface{ GraphTarget() string }); ok {
			if target := g.GraphTarget(); target != "" {
				d = d.WithGraphTarget(target)
			}
		}
	}

	if len(parserOptions) > 0 {
		d = d.WithParserOptions(parserOptions)
	}

	return d
}

// Discover performs the full discovery process.
func (d *Discovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (component.Components, error) {
	if len(d.configFilenames) == 0 {
		d.configFilenames = DefaultConfigFilenames
	}

	if d.discoveryContext == nil {
		d.discoveryContext = &component.DiscoveryContext{
			WorkingDir: d.workingDir,
		}
	}

	classifier := filter.NewClassifier(l)
	if err := classifier.Analyze(d.filters); err != nil {
		return nil, err
	}

	d.classifier = classifier

	discovered, candidates, phaseErrs := d.runFilesystemPhase(ctx, l, opts)
	if len(phaseErrs) > 0 && !d.suppressParseErrors {
		return nil, errors.Join(phaseErrs...)
	}

	if d.requiresParse || classifier.HasParseRequiredFilters() {
		discovered, candidates, phaseErrs = d.runParsePhase(ctx, l, opts, discovered, candidates)
		if len(phaseErrs) > 0 && !d.suppressParseErrors {
			return nil, errors.Join(phaseErrs...)
		}
	}

	if classifier.HasGraphFilters() {
		if classifier.HasDependentFilters() && d.gitRoot == "" {
			if gitRootPath, gitErr := shell.GitTopLevelDir(ctx, l, opts, d.workingDir); gitErr == nil {
				d.gitRoot = gitRootPath
				l.Debugf("Set gitRoot for dependent discovery: %s", d.gitRoot)
			}
		}

		discovered, _, phaseErrs = d.runGraphPhase(ctx, l, opts, discovered, candidates)
		if len(phaseErrs) > 0 && !d.suppressParseErrors {
			return nil, errors.Join(phaseErrs...)
		}
	}

	components := resultsToComponents(discovered)

	if d.discoverRelationships {
		components, phaseErrs = d.runRelationshipPhase(ctx, l, opts, components)
		if len(phaseErrs) > 0 && !d.suppressParseErrors {
			return components, errors.Join(phaseErrs...)
		}
	}

	if len(d.filters) > 0 {
		filtered, err := d.filters.Evaluate(l, components)
		if err != nil {
			return components, err
		}

		components = filtered
	}

	if _, cycleErr := components.CycleCheck(); cycleErr != nil {
		l.Debugf("Cycle: %v", cycleErr)

		if d.breakCycles {
			l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

			components = removeCycles(components)
		}
	}

	if d.graphTarget != "" {
		var err error

		components, err = d.filterGraphTarget(components)
		if err != nil {
			return nil, err
		}
	}

	components = d.applyQueueFilters(opts, components)

	return components, nil
}

// runFilesystemPhase runs the filesystem and worktree phases concurrently.
func (d *Discovery) runFilesystemPhase(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) ([]DiscoveryResult, []DiscoveryResult, []error) {
	var (
		allDiscovered []DiscoveryResult
		allCandidates []DiscoveryResult
		allErrors     []error
		mu            sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(2) //nolint:mnd

	g.Go(func() error {
		phase := NewFilesystemPhase(d.numWorkers)
		output := phase.Run(ctx, &PhaseInput{
			Logger:     l,
			Opts:       opts,
			Classifier: d.classifier,
			Discovery:  d,
		})

		discovered, candidates, errs := mergeResults(output)

		mu.Lock()

		allDiscovered = append(allDiscovered, discovered...)
		allCandidates = append(allCandidates, candidates...)
		allErrors = append(allErrors, errs...)

		mu.Unlock()

		return nil
	})

	if len(d.gitExpressions) > 0 && d.worktrees != nil {
		g.Go(func() error {
			phase := NewWorktreePhase(d.gitExpressions, d.numWorkers)
			output := phase.Run(ctx, &PhaseInput{
				Logger:     l,
				Opts:       opts,
				Classifier: d.classifier,
				Discovery:  d,
			})

			discovered, candidates, errs := mergeResults(output)

			mu.Lock()

			allDiscovered = append(allDiscovered, discovered...)
			allCandidates = append(allCandidates, candidates...)
			allErrors = append(allErrors, errs...)

			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		allErrors = append(allErrors, err)
	}

	allDiscovered = deduplicateResults(allDiscovered)
	allCandidates = deduplicateResults(allCandidates)

	return allDiscovered, allCandidates, allErrors
}

// runParsePhase runs the parse phase for candidates that require parsing.
func (d *Discovery) runParsePhase(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovered []DiscoveryResult,
	candidates []DiscoveryResult,
) ([]DiscoveryResult, []DiscoveryResult, []error) {
	phase := NewParsePhase(d.numWorkers)
	output := phase.Run(ctx, &PhaseInput{
		Logger:     l,
		Opts:       opts,
		Components: resultsToComponents(discovered),
		Candidates: candidates,
		Classifier: d.classifier,
		Discovery:  d,
	})

	newDiscovered, newCandidates, errs := mergeResults(output)

	allDiscovered := discovered
	allDiscovered = append(allDiscovered, newDiscovered...)
	allDiscovered = deduplicateResults(allDiscovered)

	return allDiscovered, newCandidates, errs
}

// runGraphPhase runs the graph traversal phase.
func (d *Discovery) runGraphPhase(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovered []DiscoveryResult,
	candidates []DiscoveryResult,
) ([]DiscoveryResult, []DiscoveryResult, []error) {
	if d.classifier.HasDependentFilters() {
		allComponents := resultsToComponents(discovered)
		allComponents = append(allComponents, resultsToComponents(candidates)...)

		buildErrs := d.buildDependencyGraph(ctx, l, opts, allComponents)
		if len(buildErrs) > 0 && !d.suppressParseErrors {
			return discovered, candidates, buildErrs
		}
	}

	phase := NewGraphPhase(d.numWorkers, d.maxDependencyDepth)
	output := phase.Run(ctx, &PhaseInput{
		Logger:     l,
		Opts:       opts,
		Components: resultsToComponents(discovered),
		Candidates: candidates,
		Classifier: d.classifier,
		Discovery:  d,
	})

	newDiscovered, newCandidates, errs := mergeResults(output)

	allDiscovered := discovered
	allDiscovered = append(allDiscovered, newDiscovered...)
	allDiscovered = deduplicateResults(allDiscovered)

	return allDiscovered, newCandidates, errs
}

// runRelationshipPhase runs the relationship discovery phase.
func (d *Discovery) runRelationshipPhase(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	components component.Components,
) (component.Components, []error) {
	phase := NewRelationshipPhase(d.numWorkers, d.maxDependencyDepth)
	output := phase.Run(ctx, &PhaseInput{
		Logger:     l,
		Opts:       opts,
		Components: components,
		Discovery:  d,
	})

	var errs []error

	<-output.Done

	for err := range output.Errors {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return components, errs
}

// buildDependencyGraph parses all components and builds bidirectional dependency links.
// This is called before the graph phase when dependent filters exist, to populate
// the reverse links (dependents) that the graph phase needs for dependent traversal.
func (d *Discovery) buildDependencyGraph(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	allComponents component.Components,
) []error {
	threadSafeComponents := component.NewThreadSafeComponents(allComponents)

	var (
		errs []error
		mu   sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(d.numWorkers)

	for _, c := range allComponents {
		g.Go(func() error {
			err := d.buildComponentDependencies(ctx, l, opts, c, threadSafeComponents)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()
			}

			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		l.Debugf("Error building dependency graph: %v", err)
	}

	return errs
}

// buildComponentDependencies parses a single component and builds its dependency links.
func (d *Discovery) buildComponentDependencies(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	c component.Component,
	threadSafeComponents *component.ThreadSafeComponents,
) error {
	unit, ok := c.(*component.Unit)
	if !ok {
		return nil
	}

	cfg := unit.Config()
	if cfg == nil {
		err := parseComponent(c, ctx, l, opts, d.suppressParseErrors, d.parserOptions)
		if err != nil {
			if d.suppressParseErrors {
				l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)
				return nil
			}

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

	parentCtx := c.DiscoveryContext()
	if parentCtx == nil {
		return nil
	}

	for _, depPath := range depPaths {
		existing := threadSafeComponents.FindByPath(depPath)
		if existing != nil {
			c.AddDependency(existing)
			continue
		}

		depComponent := component.NewUnit(depPath)

		if isExternal(parentCtx.WorkingDir, depPath) {
			depComponent.SetExternal()
		}

		addedComponent, created := threadSafeComponents.EnsureComponent(depComponent)
		if created {
			copiedCtx := parentCtx.CopyWithNewOrigin(component.OriginGraphDiscovery)
			depComponent.SetDiscoveryContext(copiedCtx)
		}

		c.AddDependency(addedComponent)
	}

	return nil
}

// removeCycles removes cycles from the dependency graph.
func removeCycles(components component.Components) component.Components {
	var (
		c   component.Component
		err error
	)

	for range maxCycleRemovalAttempts {
		c, err = components.CycleCheck()
		if err == nil {
			break
		}

		if c == nil {
			break
		}

		components = components.RemoveByPath(c.Path())
	}

	return components
}

// filterGraphTarget prunes components to the target path and its dependents.
func (d *Discovery) filterGraphTarget(components component.Components) (component.Components, error) {
	if d.graphTarget == "" {
		return components, nil
	}

	targetPath, err := canonicalizeGraphTarget(d.workingDir, d.graphTarget)
	if err != nil {
		return nil, err
	}

	dependentUnits := buildDependentsIndex(components)
	propagateTransitiveDependents(dependentUnits)

	allowed := buildAllowSet(targetPath, dependentUnits)

	return filterByAllowSet(components, allowed), nil
}

// canonicalizeGraphTarget resolves the graph target to an absolute, cleaned path with symlinks resolved.
// Returns an error if the path cannot be made absolute.
func canonicalizeGraphTarget(baseDir, target string) (string, error) {
	var abs string

	// If already absolute, just clean it
	if filepath.IsAbs(target) {
		abs = filepath.Clean(target)
	} else if canonicalAbs, err := util.CanonicalPath(target, baseDir); err == nil {
		// Try canonical path first
		abs = canonicalAbs
	} else {
		// Fallback: join with baseDir and make absolute
		joined := filepath.Join(baseDir, filepath.Clean(target))

		var absErr error

		abs, absErr = filepath.Abs(joined)
		if absErr != nil {
			return "", errors.Errorf("failed to resolve graph target %q relative to %q: %w", target, baseDir, absErr)
		}
	}

	// Resolve symlinks for consistent path comparison (important on macOS where /var -> /private/var)
	resolved, evalErr := filepath.EvalSymlinks(abs)
	if evalErr != nil {
		// If symlink resolution fails (e.g., path doesn't exist yet), return the absolute path
		return abs, nil //nolint:nilerr // intentionally return nil error when EvalSymlinks fails
	}

	return resolved, nil
}

// buildDependentsIndex builds an index mapping each unit path to the list of units
// that directly depend on it. Duplicate entries are removed.
// Paths are resolved to handle symlinks consistently across platforms.
func buildDependentsIndex(components component.Components) map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, c := range components {
		cPath := util.ResolvePath(c.Path())

		for _, dep := range c.Dependencies() {
			depPath := util.ResolvePath(dep.Path())
			dependentUnits[depPath] = util.RemoveDuplicates(append(dependentUnits[depPath], cPath))
		}
	}

	return dependentUnits
}

// propagateTransitiveDependents expands the dependents index to include transitive dependents.
// Iteratively propagates dependents until a fixed point is reached or the iteration cap is met.
func propagateTransitiveDependents(dependentUnits map[string][]string) {
	// Determine an upper bound on iterations based on unique nodes in the graph (keys + values).
	nodes := make(map[string]struct{})
	for unit, dependents := range dependentUnits {
		nodes[unit] = struct{}{}
		for _, dep := range dependents {
			nodes[dep] = struct{}{}
		}
	}

	maxIterations := len(nodes)

	for range maxIterations {
		updated := false

		for unit, dependents := range dependentUnits {
			for _, dep := range dependents {
				old := dependentUnits[unit]
				newList := util.RemoveDuplicates(append(old, dependentUnits[dep]...))
				newList = slices.DeleteFunc(newList, func(path string) bool { return path == unit })

				if len(newList) != len(old) {
					dependentUnits[unit] = newList
					updated = true
				}
			}
		}

		if !updated {
			break
		}
	}
}

// buildAllowSet creates the allowlist containing the target and all of its dependents.
func buildAllowSet(targetPath string, dependentUnits map[string][]string) map[string]struct{} {
	allowed := make(map[string]struct{})

	allowed[targetPath] = struct{}{}
	for _, dep := range dependentUnits[targetPath] {
		allowed[dep] = struct{}{}
	}

	return allowed
}

// filterByAllowSet returns only the components whose path exists in the allow set.
// Paths are resolved to handle symlinks consistently across platforms.
// The output order matches the input order (no sorting is performed here).
func filterByAllowSet(components component.Components, allowed map[string]struct{}) component.Components {
	filtered := make(component.Components, 0, len(components))

	for _, c := range components {
		resolvedPath := util.ResolvePath(c.Path())
		if _, ok := allowed[resolvedPath]; ok {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

// applyQueueFilters marks discovered units as excluded or included based on queue-related CLI flags and config.
// The runner consumes the exclusion markers instead of re-evaluating the filters.
func (d *Discovery) applyQueueFilters(opts *options.TerragruntOptions, components component.Components) component.Components {
	components = d.applyExcludeModules(opts, components)

	return components
}

// applyExcludeModules marks units (and optionally their dependencies) excluded via terragrunt exclude blocks.
func (d *Discovery) applyExcludeModules(opts *options.TerragruntOptions, components component.Components) component.Components {
	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		cfg := unit.Config()
		if cfg == nil || cfg.Exclude == nil {
			continue
		}

		if !cfg.Exclude.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if cfg.Exclude.If {
			unit.SetExcluded(true)
		}

		if cfg.Exclude.ExcludeDependencies != nil && *cfg.Exclude.ExcludeDependencies {
			for _, dep := range unit.Dependencies() {
				depUnit, ok := dep.(*component.Unit)
				if !ok {
					continue
				}

				depUnit.SetExcluded(true)
			}
		}
	}

	return components
}
