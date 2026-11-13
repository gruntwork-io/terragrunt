package discovery

import (
	"context"
	"path/filepath"
	"slices"
	"sync"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"

	"golang.org/x/sync/errgroup"
)

// Discover performs the full discovery process. The supplied [venv.Venv]
// supplies the process executor for the git top-level probe and the
// auth-provider-command credentials fetch during the parse phase.
//
// The config rewrite memoizes its parses in s when [rewrite.EnvRewrite] selects it. Discoveries that share s share
// those parses.
func (d *Discovery) Discover(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
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
		// resolve outside it (see the graph phase).
		d.discoveryBoundary = boundary

		l.Debugf("Discovery: graph traversal bounded to %s", boundary)
	}

	withWorktree := len(d.gitExpressions) > 0 && d.worktrees != nil

	if withWorktree {
		if gitRoot, gitErr := git.GoRepoRoot(ctx, v, d.workingDir); gitErr == nil {
			d.worktreeGitRoot = gitRoot
		}

		// A boundary that exists at neither compared reference is a mistake, not an empty result.
		err := CheckWorktreeBoundaries(ctx, v, d.worktrees, d.filters, d.discoveryBoundaryInput, d.workingDir)
		if err != nil {
			return nil, err
		}
	}

	found, err := d.walkAndParse(ctx, l, v, s, opts, withWorktree)
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
			"Discovery: starting graph phase (discovered=%d, candidates=%d, max_depth=%d, has_dependent_filters=%t)",
			len(
				discovered,
			),
			len(candidates),
			d.maxDependencyDepth,
			d.classifier.HasDependentFilters(),
		)

		var results *PhaseResults

		err = telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_phase_graph", map[string]any{
				"num_workers":           d.numWorkers,
				"max_dependency_depth":  d.maxDependencyDepth,
				"discovered_in":         len(discovered),
				"candidates_in":         len(candidates),
				"has_dependent_filters": d.classifier.HasDependentFilters(),
			}, func(childCtx context.Context, l log.Logger) error {
				var phaseErr error

				results, phaseErr = d.runGraphPhase(childCtx, l, v, s, opts, found)

				return phaseErr
			})

		logPhaseComplete(l, "graph", results, err)

		if err != nil && !d.suppressParseErrors {
			return nil, d.markDeletedDependency(v.FS, err, resultsToComponents(slices.Concat(discovered, candidates)))
		}

		discovered = results.Discovered
	}

	components := resultsToComponents(discovered)

	if d.discoverRelationships {
		l.Debugf("Discovery: starting relationship phase (components=%d, max_depth=%d)",
			len(components), d.maxDependencyDepth)

		err = telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_phase_relationship", map[string]any{
				"num_workers":          d.numWorkers,
				"max_dependency_depth": d.maxDependencyDepth,
				"components_in":        len(components),
			}, func(childCtx context.Context, l log.Logger) error {
				if found.relations != nil {
					return found.relations.link(childCtx, l, v, components)
				}

				var phaseErr error

				components, phaseErr = d.runRelationshipPhase(childCtx, l, v, s, opts, components)

				return phaseErr
			})

		l.Debugf(
			"Discovery: relationship phase complete (components=%d, err=%v)",
			len(components),
			err,
		)

		if err != nil && !d.suppressParseErrors {
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
				storeStackConfigs(childCtx, childL, v, opts, components)

				return nil
			}); err != nil {
			return components, err
		}
	}

	return components, nil
}

// walked is what [Discovery.walkAndParse] found.
type walked struct {
	// relations is the dependencies the pipeline read from the components, nil when the phases ran in its place or
	// discovery needs none.
	relations *pipelineRelations
	// discovered is the components ready for filtering.
	discovered []DiscoveryResult
	// candidates is the components left for the graph phase.
	candidates []DiscoveryResult
}

// walkAndParse finds the components below the working directory and parses the ones that need it, through the
// pipeline when [EnvPipeline] selects it, and through the filesystem and parse phases otherwise. With withWorktree
// it also finds the components the Git filters select in their worktrees.
func (d *Discovery) walkAndParse(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	withWorktree bool,
) (walked, error) {
	parseReasons := slices.Clone(d.parseReasons)
	if d.classifier.HasParseRequiredFilters() {
		if !slices.Contains(parseReasons, parseReasonClassifierRequiresParse) {
			parseReasons = append(parseReasons, parseReasonClassifierRequiresParse)
		}
	}

	if !PipelineEnabled(v.Env) {
		return d.runWalkAndParsePhases(ctx, l, v, s, opts, withWorktree, parseReasons)
	}

	if withWorktree {
		return d.runPipelineBesideWorktrees(ctx, l, v, s, opts, parseReasons)
	}

	return d.runTracedPipeline(ctx, l, v, s, opts, parseReasons)
}

// runPipelineBesideWorktrees runs the pipeline over the working directory while the worktree phase discovers the
// components the Git filters select, then parses the ones from the worktrees that need it with the parse phase.
func (d *Discovery) runPipelineBesideWorktrees(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	parseReasons []parseReason,
) (walked, error) {
	var (
		found        walked
		pipelineErr  error
		inWorktrees  *PhaseResults
		worktreesErr error
		walks        errgroup.Group
	)

	walks.Go(func() error {
		found, pipelineErr = d.runTracedPipeline(ctx, l, v, s, opts, parseReasons)

		return nil
	})

	walks.Go(func() error {
		inWorktrees, worktreesErr = d.runWorktreeWalk(ctx, l, v, s, opts)

		return nil
	})

	// Neither walk returns its error to the group: each records its own.
	if err := walks.Wait(); err != nil {
		panic(err)
	}

	if pipelineErr != nil {
		return walked{}, pipelineErr
	}

	if worktreesErr != nil && (!d.suppressParseErrors || errors.As(worktreesErr, new(CoexistenceError))) {
		return walked{}, worktreesErr
	}

	if inWorktrees == nil {
		return found, nil
	}

	if err := validateNoCoexistence(inWorktrees.Discovered); err != nil {
		return walked{}, err
	}

	if err := validateNoCoexistence(inWorktrees.Candidates); err != nil {
		return walked{}, err
	}

	inWorktrees.Discovered = deduplicateResults(inWorktrees.Discovered)
	inWorktrees.Candidates = deduplicateResults(inWorktrees.Candidates)

	if len(parseReasons) > 0 {
		parsed, err := d.runParsePhase(ctx, l, v, s, opts, inWorktrees.Discovered, inWorktrees.Candidates)
		if err != nil && !d.suppressParseErrors {
			return walked{}, err
		}

		inWorktrees = parsed
	}

	found.discovered = deduplicateResults(append(found.discovered, inWorktrees.Discovered...))
	found.candidates = deduplicateResults(append(found.candidates, inWorktrees.Candidates...))

	return found, nil
}

// runTracedPipeline runs the pipeline inside its telemetry span.
func (d *Discovery) runTracedPipeline(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	parseReasons []parseReason,
) (walked, error) {
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

	logPhaseComplete(l, "pipeline", &PhaseResults{Discovered: found.discovered, Candidates: found.candidates}, err)

	return found, err
}

// runWalkAndParsePhases runs the filesystem phase, then the parse phase when parseReasons names a reason for it.
func (d *Discovery) runWalkAndParsePhases(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	withWorktree bool,
	parseReasons []parseReason,
) (walked, error) {
	var (
		results *PhaseResults
		err     error
	)

	l.Debugf(
		"Discovery: starting filesystem phase (workers=%d, with_worktree=%t)",
		d.numWorkers,
		withWorktree,
	)

	err = telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_phase_filesystem", map[string]any{
			"num_workers":   d.numWorkers,
			"with_worktree": withWorktree,
		}, func(childCtx context.Context, l log.Logger) error {
			var phaseErr error

			results, phaseErr = d.runFilesystemPhase(childCtx, l, v, s, opts)

			return phaseErr
		})

	logPhaseComplete(l, "filesystem", results, err)

	if err != nil && (!d.suppressParseErrors || errors.As(err, new(CoexistenceError))) {
		return walked{}, err
	}

	discovered, candidates := results.Discovered, results.Candidates

	if len(parseReasons) == 0 {
		return walked{discovered: discovered, candidates: candidates}, nil
	}

	reasonsStr := joinParseReasons(parseReasons)

	l.Debugf("Discovery: starting parse phase (discovered=%d, candidates=%d, reasons=%s)",
		len(discovered), len(candidates), reasonsStr)

	err = telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_phase_parse", map[string]any{
			"num_workers":       d.numWorkers,
			"discovered_in":     len(discovered),
			"candidates_in":     len(candidates),
			"parse_includes":    d.parseIncludes,
			"parse_exclude":     d.parseExclude,
			"read_files":        d.readFiles,
			"track_reads":       d.trackReads,
			"activation_reason": reasonsStr,
		}, func(childCtx context.Context, l log.Logger) error {
			var phaseErr error

			results, phaseErr = d.runParsePhase(childCtx, l, v, s, opts, discovered, candidates)

			return phaseErr
		})

	logPhaseComplete(l, "parse", results, err)

	if err != nil && !d.suppressParseErrors {
		return walked{}, err
	}

	return walked{discovered: results.Discovered, candidates: results.Candidates}, nil
}

// logPhaseComplete emits a debug log summarizing a discovery phase's outcome.
func logPhaseComplete(l log.Logger, name string, results *PhaseResults, err error) {
	var discovered, candidates int
	if results != nil {
		discovered = len(results.Discovered)
		candidates = len(results.Candidates)
	}

	l.Debugf(
		"Discovery: %s phase complete (discovered=%d, candidates=%d, err=%v)",
		name,
		discovered,
		candidates,
		err,
	)
}

// runFilesystemPhase runs the filesystem and worktree phases concurrently.
func (d *Discovery) runFilesystemPhase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) (*PhaseResults, error) {
	var (
		allDiscovered []DiscoveryResult
		allCandidates []DiscoveryResult
		allErrors     []error
		mu            sync.Mutex
	)

	// maxPhases is the maximum number of phases to run concurrently
	// for filesystem and worktree phases.
	const maxPhases = 2

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxPhases)

	g.Go(func() error {
		var result *PhaseResults

		l.Debugf("Discovery: starting filesystem walk at %s", d.workingDir)

		err := telemetry.TelemeterFromContext(ctx).
			Collect(ctx, l, "discovery_filesystem_walk", map[string]any{
				"num_workers": d.numWorkers,
				"working_dir": d.workingDir,
			}, func(childCtx context.Context, l log.Logger) error {
				phase := NewFilesystemPhase(d.numWorkers)

				var phaseErr error

				result, phaseErr = phase.Run(childCtx, l, v, s, &PhaseInput{
					Opts:       opts,
					Classifier: d.classifier,
					Discovery:  d,
				})

				return phaseErr
			})

		logPhaseComplete(l, "filesystem walk", result, err)

		mu.Lock()

		if result != nil {
			allDiscovered = append(allDiscovered, result.Discovered...)
			allCandidates = append(allCandidates, result.Candidates...)
		}

		if err != nil {
			allErrors = append(allErrors, err)
		}

		mu.Unlock()

		return nil
	})

	if len(d.gitExpressions) > 0 && d.worktrees != nil {
		g.Go(func() error {
			result, err := d.runWorktreeWalk(ctx, l, v, s, opts)

			mu.Lock()

			if result != nil {
				allDiscovered = append(allDiscovered, result.Discovered...)
				allCandidates = append(allCandidates, result.Candidates...)
			}

			if err != nil {
				allErrors = append(allErrors, err)
			}

			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		allErrors = append(allErrors, err)
	}

	if err := validateNoCoexistence(allDiscovered); err != nil {
		return nil, err
	}

	if err := validateNoCoexistence(allCandidates); err != nil {
		return nil, err
	}

	allDiscovered = deduplicateResults(allDiscovered)
	allCandidates = deduplicateResults(allCandidates)

	return &PhaseResults{
		Discovered: allDiscovered,
		Candidates: allCandidates,
	}, errors.Join(allErrors...)
}

// runWorktreeWalk discovers the components the Git filters select in their worktrees.
func (d *Discovery) runWorktreeWalk(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) (*PhaseResults, error) {
	var result *PhaseResults

	l.Debugf(
		"Discovery: starting worktree walk (git_expressions=%d)",
		len(d.gitExpressions),
	)

	err := telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_worktree_walk", map[string]any{
			"num_workers":          d.numWorkers,
			"git_expression_count": len(d.gitExpressions),
		}, func(childCtx context.Context, l log.Logger) error {
			phase := NewWorktreePhase(d.gitExpressions, d.numWorkers)

			var phaseErr error

			result, phaseErr = phase.Run(childCtx, l, v, s, &PhaseInput{
				Opts:       opts,
				Classifier: d.classifier,
				Discovery:  d,
			})

			return phaseErr
		})

	logPhaseComplete(l, "worktree walk", result, err)

	return result, err
}

// runParsePhase runs the parse phase for candidates that require parsing.
func (d *Discovery) runParsePhase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	discovered []DiscoveryResult,
	candidates []DiscoveryResult,
) (*PhaseResults, error) {
	phase := NewParsePhase(d.numWorkers)
	result, err := phase.Run(ctx, l, v, s, &PhaseInput{
		Opts:       opts,
		Components: resultsToComponents(discovered),
		Candidates: candidates,
		Classifier: d.classifier,
		Discovery:  d,
	})

	allDiscovered := discovered
	if result != nil {
		allDiscovered = append(allDiscovered, result.Discovered...)
	}

	allDiscovered = deduplicateResults(allDiscovered)

	var resultCandidates []DiscoveryResult
	if result != nil {
		resultCandidates = result.Candidates
	}

	return &PhaseResults{
		Discovered: allDiscovered,
		Candidates: resultCandidates,
	}, err
}

// runGraphPhase runs the graph traversal phase over what found holds. When a filter selects dependents, it first
// links every component to its dependencies. Both steps take dependencies from the ones the pipeline read when
// found has them.
func (d *Discovery) runGraphPhase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	found walked,
) (*PhaseResults, error) {
	discovered, candidates := found.discovered, found.candidates

	if d.classifier.HasDependentFilters() {
		allComponents := resultsToComponents(discovered)
		allComponents = append(allComponents, resultsToComponents(candidates)...)

		unparsed := d.potentialDependentsOutsideBoundary(l, v.FS, candidates)

		buildErr := telemetry.TelemeterFromContext(ctx).Collect(
			ctx, l, "discover_dependents", map[string]any{},
			func(childCtx context.Context, l log.Logger) error {
				if found.relations != nil {
					return errors.Join(found.relations.linkForDependents(childCtx, l, v, allComponents, unparsed)...)
				}

				return errors.Join(d.buildDependencyGraph(childCtx, l, v, s, opts, allComponents, unparsed)...)
			})

		if buildErr != nil && !d.suppressParseErrors {
			return &PhaseResults{
				Discovered: discovered,
				Candidates: candidates,
			}, buildErr
		}
	}

	phase := NewGraphPhase(d.numWorkers, d.maxDependencyDepth)
	phase.relations = found.relations

	var result *PhaseResults

	err := telemetry.TelemeterFromContext(ctx).Collect(
		ctx, l, "discover_dependencies", map[string]any{},
		func(childCtx context.Context, l log.Logger) error {
			var runErr error

			result, runErr = phase.Run(childCtx, l, v, s, &PhaseInput{
				Opts:       opts,
				Components: resultsToComponents(discovered),
				Candidates: candidates,
				Classifier: d.classifier,
				Discovery:  d,
			})

			return runErr
		})

	allDiscovered := discovered
	if result != nil {
		allDiscovered = append(allDiscovered, result.Discovered...)
	}

	allDiscovered = deduplicateResults(allDiscovered)

	var resultCandidates []DiscoveryResult
	if result != nil {
		resultCandidates = result.Candidates
	}

	return &PhaseResults{
		Discovered: allDiscovered,
		Candidates: resultCandidates,
	}, err
}

// runRelationshipPhase runs the relationship discovery phase.
func (d *Discovery) runRelationshipPhase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	components component.Components,
) (component.Components, error) {
	phase := NewRelationshipPhase(d.numWorkers, d.maxDependencyDepth)
	_, err := phase.Run(ctx, l, v, s, &PhaseInput{
		Opts:       opts,
		Components: components,
		Discovery:  d,
	})

	return components, err
}

// buildDependencyGraph parses all components and builds bidirectional dependency links.
// This is called before the graph phase when dependent filters exist, to populate
// the reverse links (dependents) that the graph phase needs for dependent traversal.
// Components whose paths are in unparsed stay in the graph but are not parsed.
func (d *Discovery) buildDependencyGraph(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	allComponents component.Components,
	unparsed map[string]struct{},
) []error {
	threadSafeComponents := component.NewThreadSafeComponents(d.paths, allComponents)

	var (
		errs []error
		mu   sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(d.numWorkers)

	for _, c := range allComponents {
		if _, skip := unparsed[c.Path()]; skip {
			l.Debugf("Discovery: %s is outside every dependent boundary; not parsing it", c.Path())
			continue
		}

		g.Go(func() error {
			err := d.buildComponentDependencies(ctx, l, v, s, opts, c, threadSafeComponents)
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
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	c component.Component,
	threadSafeComponents *component.ThreadSafeComponents,
) error {
	unit, ok := c.(*component.Unit)
	if !ok {
		return nil
	}

	ctx = contextWithParsePhase(ctx, parsePhaseTagDependencyGraph)

	if err := ensureParsed(ctx, l, v, s, c, opts, d); err != nil {
		if d.suppressParseErrors {
			l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)
			return nil
		}

		return err
	}

	cfg := unit.Config()

	depPaths, err := extractDependencyPaths(d.paths, cfg, c)
	if err != nil {
		return err
	}

	depPaths, err = stackDependencyPaths(ctx, l, v, opts, depPaths)
	if err != nil {
		return err
	}

	linkGraphDependencies(v.FS, c, depPaths, threadSafeComponents)

	return nil
}

// linkGraphDependencies links c to the component at each of depPaths, taking it from components or adding it there
// as one the graph reached from c.
func linkGraphDependencies(
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
		depComponent := componentFromDependencyPath(fsys, depPath, components)

		if dctx := depComponent.DiscoveryContext(); dctx == nil || dctx.WorkingDir == "" {
			depComponent.SetDiscoveryContext(
				parentCtx.CopyWithNewOrigin(component.OriginGraphDiscovery),
			)

			if isExternal(fsys, parentCtx.WorkingDir, depPath) {
				if ext, ok := depComponent.(*component.Unit); ok {
					ext.SetExternal()
				}
			}
		}

		addedComponent, _ := components.EnsureComponent(depComponent)

		c.AddDependency(addedComponent)
	}
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
		if !reachedByTraversal(c) {
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

// reachedByTraversal reports whether a component entered discovery by following
// the dependency graph rather than by being walked to.
func reachedByTraversal(c component.Component) bool {
	dctx := c.DiscoveryContext()
	if dctx == nil {
		return false
	}

	origin := dctx.Origin()

	return origin == component.OriginGraphDiscovery ||
		origin == component.OriginRelationshipDiscovery
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
