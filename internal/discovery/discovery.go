package discovery

import (
	"context"
	"path/filepath"
	"slices"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Discover performs the full discovery process. The supplied [venv.Venv]
// supplies the process executor for the git top-level probe and the
// auth-provider-command credentials fetch during the parses.
//
// The parses of units are memoized in s. Discoveries that share s share those parses.
func (d *Discovery) Discover(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
) (component.Components, error) {
	d.classifier = filter.NewClassifier(d.filters)
	d.paths = vfs.NewPathResolver(v.FS)

	defer func() { tips.GiveInputsReadTip(l, v, d.InputsReads(), opts.Tips) }()

	// A working directory that cannot be walked to discovers nothing, which is
	// reported as an empty result rather than an error, so it keeps the
	// spelling it was given and comparisons against it stay consistent.
	resolvedWorkingDir, resolveErr := resolveDir(v.FS, d.workingDir)
	if resolveErr != nil {
		resolvedWorkingDir = filepath.Clean(d.workingDir)
	}

	d.resolvedWorkingDir = resolvedWorkingDir

	l.Debugf("Discovery: %d filter(s) configured: %s", len(d.filters), d.filters)

	if d.discoveryBoundaryInput == "" {
		d.discoveryBoundaryInput = d.discoveryBoundary
	}

	if d.discoveryBoundary != "" {
		boundary, boundaryErr := resolveDiscoveryBoundary(
			v.FS,
			d.workingDir,
			d.discoveryBoundary,
			boundaryEnclosureFor(d.filters),
		)
		if boundaryErr != nil {
			return nil, boundaryErr
		}

		// Store the resolved enclosure. It caps the dependent walk in place of
		// the git root (see dependentWalkBoundary) and prunes dependencies that
		// resolve outside it (see the graph traversal).
		d.discoveryBoundary = boundary

		l.Debugf("Discovery: graph traversal bounded to %s", boundary)
	}

	if d.findsInWorktrees() {
		if gitRoot, gitErr := git.GoRepoRoot(ctx, v, d.workingDir); gitErr == nil {
			d.worktreeGitRoot = gitRoot
		}

		// A boundary that exists at neither compared reference is a mistake, not an empty result.
		err := CheckWorktreeBoundaries(ctx, v, d.worktrees, d.filters, d.discoveryBoundaryInput, d.workingDir)
		if err != nil {
			return nil, err
		}
	}

	found, err := d.walkAndParse(ctx, l, v, s, opts)
	if err != nil {
		return nil, err
	}

	discovered, candidates := found.discovered, found.candidates

	if d.classifier.HasGraphFilters() {
		if d.classifier.HasDependentFilters() && d.discoveryBoundary == "" && d.gitRoot == "" {
			if gitRootPath, gitErr := git.GoRepoRoot(ctx, v, d.workingDir); gitErr == nil {
				d.gitRoot = gitRootPath
				l.Debugf("Set dependent discovery boundary to git root: %s", d.gitRoot)
			}
		}

		l.Debugf(
			"Discovery: starting graph traversal (discovered=%d, candidates=%d, max_depth=%d, has_dependent_filters=%t)",
			len(
				discovered,
			),
			len(candidates),
			d.maxDependencyDepth,
			d.classifier.HasDependentFilters(),
		)

		var results *foundResults

		err = telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_phase_graph", map[string]any{
				"num_workers":           d.numWorkers,
				"max_dependency_depth":  d.maxDependencyDepth,
				"discovered_in":         len(discovered),
				"candidates_in":         len(candidates),
				"has_dependent_filters": d.classifier.HasDependentFilters(),
			}, func(childCtx context.Context, l log.Logger) error {
				var phaseErr error

				results, phaseErr = d.traverseGraph(childCtx, l, v, opts, found)

				return phaseErr
			})

		logComplete(l, "graph traversal", results, err)

		if d.returnsPhaseError(err) {
			return nil, d.markDeletedDependency(v.FS, err, resultsToComponents(slices.Concat(discovered, candidates)))
		}

		discovered = results.discovered
	}

	components := resultsToComponents(discovered)

	if d.discoverRelationships {
		l.Debugf("Discovery: starting relationships (components=%d, max_depth=%d)",
			len(components), d.maxDependencyDepth)

		err = telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_phase_relationship", map[string]any{
				"num_workers":          d.numWorkers,
				"max_dependency_depth": d.maxDependencyDepth,
				"components_in":        len(components),
			}, func(childCtx context.Context, l log.Logger) error {
				return found.relations.link(childCtx, l, v, components)
			})

		l.Debugf(
			"Discovery: relationships complete (components=%d, err=%v)",
			len(components),
			err,
		)

		if d.returnsPhaseError(err) {
			return components, d.markDeletedDependency(v.FS, err, components)
		}
	}

	if len(d.filters) > 0 {
		l.Debugf(
			"Discovery: applying %d filter(s) to %d components",
			len(d.filters),
			len(components),
		)

		filtered, err := d.filters.Evaluate(l, d.evaluationContext(), components)
		if err != nil {
			return components, err
		}

		l.Debugf(
			"Discovery: filter evaluation complete (in=%d, out=%d)",
			len(components),
			len(filtered),
		)

		components = filtered
	}

	components = d.dropOutsideBoundary(l, v.FS, components)

	cycleCheckErr := telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_cycle_check", map[string]any{},
			func(childCtx context.Context, l log.Logger) error {
				if _, cycleErr := components.CycleCheck(); cycleErr != nil {
					l.Debugf("Cycle: %v", cycleErr)

					if d.breakCycles {
						l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

						var removeErr error

						components, removeErr = removeCycles(components)
						if removeErr != nil {
							return removeErr
						}
					}
				}

				return nil
			})

	if cycleCheckErr != nil && !d.suppressParseErrors {
		return components, cycleCheckErr
	}

	if d.graphTarget != "" {
		components = d.filterGraphTarget(v.FS, components)
	}

	components = d.applyQueueFilters(opts, components)

	if d.parseStackConfigs {
		if err := telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_phase_stack_configs", map[string]any{
				"components_in": len(components),
			}, func(childCtx context.Context, childL log.Logger) error {
				storeStackConfigs(childCtx, childL, v, s, opts, components)

				return nil
			}); err != nil {
			return components, err
		}
	}

	return components, nil
}

// walked is what walkAndParse found.
type walked struct {
	// relations is the dependencies the pipeline read from the components, nil when discovery needs none.
	relations *pipelineRelations
	// discovered is the components ready for filtering.
	discovered []DiscoveryResult
	// candidates is the components left for the graph traversal.
	candidates []DiscoveryResult
}

// walkAndParse runs the pipeline inside its telemetry span. The pipeline finds the components below the working
// directory and the ones the Git filters select in their worktrees, and parses the ones that need it.
func (d *Discovery) walkAndParse(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
) (walked, error) {
	parseReasons := slices.Clone(d.parseReasons)
	if d.classifier.HasParseRequiredFilters() {
		if !slices.Contains(parseReasons, parseReasonClassifierRequiresParse) {
			parseReasons = append(parseReasons, parseReasonClassifierRequiresParse)
		}
	}

	l.Debugf("Discovery: starting pipeline (workers=%d, parse_reasons=%s)", d.numWorkers, joinParseReasons(parseReasons))

	var found walked

	err := telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_pipeline", map[string]any{
			"num_workers":       d.numWorkers,
			"parse_includes":    d.parseIncludes,
			"parse_exclude":     d.parseExclude,
			"read_files":        d.readFiles,
			"track_reads":       d.trackReads,
			"activation_reason": joinParseReasons(parseReasons),
		}, func(childCtx context.Context, l log.Logger) error {
			var pipelineErr error

			found, pipelineErr = d.runPipeline(childCtx, l, v, s, opts, len(parseReasons) > 0)

			return pipelineErr
		})

	logComplete(l, "pipeline", &foundResults{discovered: found.discovered, candidates: found.candidates}, err)

	return found, err
}

// findsInWorktrees reports whether discovery also finds the components its Git filters select in their worktrees.
func (d *Discovery) findsInWorktrees() bool {
	return len(d.gitExpressions) > 0 && d.worktrees != nil
}

// logComplete emits a debug log summarizing the outcome of the step of discovery called name. results is nil when
// the step failed before it had any.
func logComplete(l log.Logger, name string, results *foundResults, err error) {
	var discovered, candidates int
	if results != nil {
		discovered = len(results.discovered)
		candidates = len(results.candidates)
	}

	l.Debugf(
		"Discovery: %s complete (discovered=%d, candidates=%d, err=%v)",
		name,
		discovered,
		candidates,
		err,
	)
}

// runWorktreeWalk discovers the components the Git filters select in their worktrees.
func (d *Discovery) runWorktreeWalk(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
) (*foundResults, error) {
	var result *foundResults

	l.Debugf(
		"Discovery: starting worktree walk (git_expressions=%d)",
		len(d.gitExpressions),
	)

	err := telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_worktree_walk", map[string]any{
			"num_workers":          d.numWorkers,
			"git_expression_count": len(d.gitExpressions),
		}, func(childCtx context.Context, l log.Logger) error {
			var walkErr error

			result, walkErr = d.discoverInWorktrees(childCtx, l, v, s, opts)

			return walkErr
		})

	logComplete(l, "worktree walk", result, err)

	return result, err
}

// traverseGraph runs the graph traversal over what found holds. When a filter selects dependents, it first links
// every component to its dependencies. Both steps take dependencies from the ones the pipeline read.
func (d *Discovery) traverseGraph(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	found walked,
) (*foundResults, error) {
	discovered, candidates := found.discovered, found.candidates

	if d.classifier.HasDependentFilters() {
		allComponents := resultsToComponents(discovered)
		allComponents = append(allComponents, resultsToComponents(candidates)...)

		unparsed := d.potentialDependentsOutsideBoundary(l, v.FS, candidates)

		buildErr := telemetry.TelemeterFromContext(ctx).Collect(
			ctx, l, "discover_dependents", map[string]any{},
			func(childCtx context.Context, l log.Logger) error {
				return errors.Join(found.relations.linkForDependents(childCtx, l, v, allComponents, unparsed)...)
			})

		if d.returnsPhaseError(buildErr) {
			return &foundResults{discovered: discovered, candidates: candidates}, buildErr
		}
	}

	graph := newGraphTraversal(found.relations, d.maxDependencyDepth)

	var result *foundResults

	err := telemetry.TelemeterFromContext(ctx).Collect(
		ctx, l, "discover_dependencies", map[string]any{},
		func(childCtx context.Context, l log.Logger) error {
			var runErr error

			result, runErr = graph.run(childCtx, l, v, opts, d, resultsToComponents(discovered), candidates)

			return runErr
		})

	return &foundResults{
		discovered: deduplicateResults(append(discovered, result.discovered...)),
		candidates: result.candidates,
	}, err
}

// returnsPhaseError reports whether Discover returns err from a phase: any error unless parse errors are suppressed,
// and a [CoexistenceError] either way.
func (d *Discovery) returnsPhaseError(err error) bool {
	if err == nil {
		return false
	}

	return !d.suppressParseErrors || errors.As(err, new(CoexistenceError))
}

// removeCycles removes cycles from the dependency graph.
func removeCycles(components component.Components) (component.Components, error) {
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

	return components, err
}

// filterGraphTarget prunes components to the target path and its dependents.
func (d *Discovery) filterGraphTarget(
	fsys vfs.FS,
	components component.Components,
) component.Components {
	if d.graphTarget == "" {
		return components
	}

	targetPath := canonicalizeGraphTarget(fsys, d.workingDir, d.graphTarget)

	dependentUnits := buildDependentsIndex(d.paths, components)

	allowed := buildAllowSet(targetPath, dependentUnits)

	return filterByAllowSet(d.paths, components, allowed)
}

// canonicalizeGraphTarget resolves the graph target to an absolute, cleaned path with symlinks resolved.
// Returns an error if the path cannot be made absolute.
func canonicalizeGraphTarget(fsys vfs.FS, baseDir, target string) string {
	var abs string

	// If already absolute, just clean it
	if filepath.IsAbs(target) {
		abs = filepath.Clean(target)
	} else if canonicalAbs, err := util.CanonicalPath(target, baseDir); err == nil {
		// Try canonical path first
		abs = canonicalAbs
	} else {
		// Fallback: join with baseDir and clean
		abs = filepath.Clean(filepath.Join(baseDir, target))
	}

	// Resolve symlinks for consistent path comparison (important on macOS where /var -> /private/var)
	// EvalSymlinks can fail for: non-existent paths (expected during discovery),
	// broken symlinks, or permission issues. In all cases, falling back to the
	// absolute path is acceptable - the path will be validated later when used.
	return vfs.ResolveForCompare(fsys, abs)
}

// buildDependentsIndex builds an index mapping each unit path to the list of units
// that directly depend on it. A unit that lists the same dependency twice appears twice.
// Paths are resolved to handle symlinks consistently across platforms.
func buildDependentsIndex(
	paths *vfs.PathResolver,
	components component.Components,
) map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, c := range components {
		cPath := paths.Resolve(c.Path())

		for _, dep := range c.Dependencies() {
			depPath := paths.Resolve(dep.Path())
			dependentUnits[depPath] = append(dependentUnits[depPath], cPath)
		}
	}

	return dependentUnits
}

// buildAllowSet creates the allowlist containing the target and every unit that depends on it, directly or
// transitively.
func buildAllowSet(targetPath string, dependentUnits map[string][]string) map[string]struct{} {
	allowed := map[string]struct{}{targetPath: {}}
	pending := []string{targetPath}

	for len(pending) > 0 {
		unit := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		for _, dependent := range dependentUnits[unit] {
			if _, ok := allowed[dependent]; ok {
				continue
			}

			allowed[dependent] = struct{}{}
			pending = append(pending, dependent)
		}
	}

	return allowed
}

// filterByAllowSet returns only the components whose path exists in the allow set.
// Paths are resolved to handle symlinks consistently across platforms.
// The output order matches the input order (no sorting is performed here).
func filterByAllowSet(
	paths *vfs.PathResolver,
	components component.Components,
	allowed map[string]struct{},
) component.Components {
	filtered := make(component.Components, 0, len(components))

	for _, c := range components {
		resolvedPath := paths.Resolve(c.Path())
		if _, ok := allowed[resolvedPath]; ok {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

// applyQueueFilters marks discovered units as excluded or included based on queue-related CLI flags and config.
// The runner consumes the exclusion markers instead of re-evaluating the filters.
func (d *Discovery) applyQueueFilters(
	opts *options.TerragruntOptions,
	components component.Components,
) component.Components {
	components = d.applyExcludeModules(opts, components)

	return components
}

// dropOutsideBoundary removes the components graph traversal reached across the
// discovery boundary from what discovery returns. Their configurations were
// still read and the edges pointing at them still stand, so the components that
// do run can be ordered against them and can fetch their outputs. Components the
// filesystem walk found are left alone, boundary or not: the boundary says how
// far a run may follow the graph, not where discovery starts.
func (d *Discovery) dropOutsideBoundary(
	l log.Logger,
	fsys vfs.FS,
	components component.Components,
) component.Components {
	if d.discoveryBoundary == "" {
		return components
	}

	// An inline "(dir)" operand overrides the flag for the expression carrying
	// it, and evaluation has already honored that. Applying the flag again here
	// would undo an operand that reaches wider than it.
	if d.filters.HasGraphBoundary() {
		return components
	}

	kept := make(component.Components, 0, len(components))

	for _, c := range components {
		if !component.ReachedByTraversal(c) {
			kept = append(kept, c)
			continue
		}

		// A component found in a Git worktree is bounded by the flag resolved in that worktree.
		boundary := d.discoveryBoundary
		if root := d.worktreeRootOf(fsys, c.Path()); root != "" {
			boundary = filter.WorktreeBoundaryPath(root, d.worktreeGitRoot, d.discoveryBoundaryInput)
		}

		if isExternal(fsys, boundary, c.Path()) {
			l.Debugf(
				"Discovery: %s was reached across discovery boundary %s; not returning it",
				c.Path(),
				boundary,
			)

			continue
		}

		kept = append(kept, c)
	}

	return kept
}

// applyExcludeModules marks units (and optionally their dependencies) excluded via terragrunt exclude blocks.
func (d *Discovery) applyExcludeModules(
	opts *options.TerragruntOptions,
	components component.Components,
) component.Components {
	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		cfg := unit.Config()
		if cfg == nil || cfg.Exclude == nil {
			continue
		}

		if !cfg.Exclude.Excludes(opts.TerraformCliArgs.CommandName(opts.TofuCommand)) {
			continue
		}

		unit.SetExcluded(true)

		if cfg.Exclude.ExcludeDependencies == nil || !*cfg.Exclude.ExcludeDependencies {
			continue
		}

		for _, dep := range unit.Dependencies() {
			depUnit, ok := dep.(*component.Unit)
			if !ok {
				continue
			}

			depUnit.SetExcluded(true)
		}
	}

	return components
}
