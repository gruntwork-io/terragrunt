package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// EnvPipeline is a temporary, undocumented environment variable that switches discovery from its walk and parse
// phases to the pipeline. Do NOT rely on it: it can be removed or have its name changed at any time without notice
// and is not part of Terragrunt's user-facing configuration surface.
const EnvPipeline = "TG_TMP_DISCOVERY_PIPELINE"

// workQueuePerWorker is the number of components that can wait for each parse worker before the walk blocks.
const workQueuePerWorker = 4

// PipelineEnabled reports whether env asks for the pipeline through [EnvPipeline]. A value that is not a boolean
// leaves it off.
func PipelineEnabled(env map[string]string) bool {
	venv.RequireEnvMap(env)

	enabled, err := strconv.ParseBool(env[EnvPipeline])

	return err == nil && enabled
}

// pipelineStage is the stage of the pipeline an event comes from.
type pipelineStage int

const (
	// stageWalked is the walk finding a component and classifying it without a parse.
	stageWalked pipelineStage = iota
	// stageInWorktree is the worktree phase finding a component a Git filter selects and classifying it without a
	// parse.
	stageInWorktree
	// stageParsed is a worker finishing the parse of a component the walk sent it.
	stageParsed
	// stageRelated is a worker reading the dependencies of a component whose classification needed no parse.
	stageRelated
)

// pipelineEvent is what a stage of the pipeline reports to the collector.
type pipelineEvent struct {
	// err is why the parse of result's component failed, nil when it parsed or the event is not from stageParsed.
	err error
	// declared is the dependencies result's component declares, nil when the stage read none.
	declared *dependencyPaths
	// result is the component with its classification at stage.
	result DiscoveryResult
	// stage is the stage the event comes from.
	stage pipelineStage
}

// pipeline walks the working directory and parses the components that need it while the walk is still running.
//
// The walk classifies each component, reports it on events, and queues it on work when it needs a parse. Workers
// parse what work holds, classify the component again, and report it on events. The walk also queues the targets of
// graph filters, and when a filter selects dependents, the components ready for filtering and the candidates its
// traversals can reach. A worker reads the dependencies of such a component once it is done with it.
// One collector receives every event, so the results need no lock.
//
// A discovery with Git filters has a second finder beside the walk. It runs [PhaseWorktree], then reports and
// queues the components that phase finds as the walk does its own.
//
// The pipeline ends because its channels form a chain with no way back:
//
//  1. The walk ends when the tree is exhausted, it fails, or ctx is done. The worktree phase ends when the
//     discoveries it runs in its worktrees have ended.
//  2. The finders are the only senders on work, which closes when every finder has ended.
//  3. A worker ends when work is closed and empty, so the workers end once the finders have.
//  4. The finders and the workers are the only senders on events, which closes when all of them have ended.
//  5. The collector ends when events is closed and empty.
//
// A sender gives up when ctx is done, so a full channel never holds a stage past cancellation.
type pipeline struct {
	// discovery is the discovery the pipeline runs for.
	discovery *Discovery
	// opts is the options every parse runs with.
	opts *options.TerragruntOptions
	// store memoizes the parses.
	store *rewrite.Store
	// work carries the components the finders queue for the workers.
	work chan DiscoveryResult
	// events carries what the finders and the workers report to the collector.
	events chan pipelineEvent
	// dependentBoundaries is the boundary of each filter that selects dependents, nil when any is unbounded.
	dependentBoundaries []string
	// parseRequested reports whether an option or a filter asked for a parse.
	parseRequested bool
	// relates reports whether discovery relates the components it finds.
	relates bool
	// findsDependents reports whether a filter selects dependents.
	findsDependents bool
}

// runPipeline walks the working directory and parses the components that need it, as the filesystem and parse
// phases do one after the other. It also reads the dependencies of the targets of graph filters and, when a filter
// selects dependents, of the components that filter can reach, and returns them unlinked. The relations are nil when
// discovery neither relates components nor has a graph filter. A discovery with Git filters gets the components they
// select in their worktrees as well.
//
// Returns the first of these that occurred: the error of the walk, the parse errors of what it found, the error of
// the worktree phase, and the parse errors of what that phase found. Each set of parse errors comes back joined.
// suppressParseErrors drops them all, as it does for the phases. A [CoexistenceError], a
// [config.AmbiguousConfigError], or the error of ctx once it is done is returned either way.
func (d *Discovery) runPipeline(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	parseRequested bool,
) (walked, error) {
	workers := max(d.numWorkers, defaultDiscoveryWorkers)

	p := &pipeline{
		discovery:       d,
		opts:            opts,
		store:           s,
		work:            make(chan DiscoveryResult, workers*workQueuePerWorker),
		events:          make(chan pipelineEvent, workers*workQueuePerWorker),
		parseRequested:  parseRequested,
		relates:         d.discoverRelationships,
		findsDependents: d.classifier.HasDependentFilters(),
	}

	if p.findsDependents {
		p.dependentBoundaries = d.dependentBoundaries(l, v.FS)
	}

	var (
		finders, stages       sync.WaitGroup
		walkErr, worktreesErr error
	)

	finders.Go(func() { walkErr = p.walk(ctx, l, v) })

	if d.findsInWorktrees() {
		finders.Go(func() { worktreesErr = p.findInWorktrees(ctx, l, v) })
	}

	stages.Go(func() {
		finders.Wait()
		close(p.work)
	})

	for range workers {
		stages.Go(func() { p.parse(ctx, l, v) })
	}

	drained := make(chan *collected, 1)

	go func() { drained <- p.collect() }()

	stages.Wait()

	close(p.events)

	collected := <-drained

	if found, ok := errors.AsType[conflictError](walkErr); ok {
		return walked{}, found.error
	}

	if err := ctx.Err(); err != nil {
		return walked{}, err
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(
			attribute.Int("walked", len(collected.walked.discovered)+len(collected.walked.candidates)),
			attribute.Int("in_worktrees", len(collected.inWorktrees.discovered)+len(collected.inWorktrees.candidates)),
			attribute.Int("parsed", len(collected.parsed)+len(collected.parseErrs)),
		)
	}

	results, parseErrs, err := collected.results(d, collected.walked)
	if err != nil {
		return walked{}, err
	}

	found := walked{discovered: results.Discovered, candidates: results.Candidates}

	if p.relates || d.classifier.HasGraphFilters() {
		found.relations = &pipelineRelations{discovery: d, opts: opts, store: s, declared: collected.declared}
	}

	if !d.suppressParseErrors {
		if walkErr != nil {
			return walked{}, walkErr
		}

		if len(parseErrs) > 0 {
			return walked{}, errors.Join(parseErrs...)
		}
	}

	if worktreesErr != nil {
		if d.returnsPhaseError(worktreesErr) {
			return walked{}, worktreesErr
		}

		return found, nil
	}

	inWorktrees, parseErrs, err := collected.results(d, collected.inWorktrees)
	if err != nil {
		return walked{}, err
	}

	if len(parseErrs) > 0 && !d.suppressParseErrors {
		return walked{}, errors.Join(parseErrs...)
	}

	found.discovered = deduplicateResults(append(found.discovered, inWorktrees.Discovered...))
	found.candidates = deduplicateResults(append(found.candidates, inWorktrees.Candidates...))

	return found, nil
}

// walk walks the working directory, reports each component it finds on events, and queues the ones that need a
// parse on work.
//
// Returns a conflictError when a component the walk queues for a worker shares its directory with a config file
// that validateNoCoexistence rejects beside it, before either is parsed.
func (p *pipeline) walk(ctx context.Context, l log.Logger, v *venv.Venv) error {
	d := p.discovery
	phase := NewFilesystemPhase(d.numWorkers)
	input := &PhaseInput{Opts: p.opts, Classifier: d.classifier, Discovery: d}

	filenames := d.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	visit := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.IsDir() {
			return phase.skipDirIfIgnorable(d, entry.Name())
		}

		result := phase.processFile(input, path, filenames)
		if result == nil || result.Status == filter.StatusExcluded {
			return nil
		}

		select {
		case p.events <- pipelineEvent{result: *result, stage: stageWalked}:
		case <-ctx.Done():
			return ctx.Err()
		}

		if !p.needsParse(*result) && !p.readsDependencies(v.FS, *result) {
			return nil
		}

		if err := conflict(v.FS, phase, input, *result, path, filenames); err != nil {
			return err
		}

		select {
		case p.work <- *result:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	}

	roots, reasons := d.classifier.WalkRoots(d.discoveryContext.WorkingDir)

	l.Debugf("Discovery: pruned walk from %d root(s): %s", len(roots), strings.Join(reasons, "; "))

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(
			attribute.Int("walk_roots", len(roots)),
			attribute.StringSlice("walk_reasons", reasons),
		)
	}

	walkRoot, err := walkRootUnder(v.FS, d.discoveryContext.WorkingDir, d.walkRoot)
	if err != nil {
		return err
	}

	walk := &prunedWalk{
		visit:       visit,
		workingDir:  d.discoveryContext.WorkingDir,
		walkRoot:    walkRoot,
		numWorkers:  phase.numWorkers,
		followLinks: p.opts.Experiments.Evaluate(experiment.Symlinks),
	}

	return walk.run(ctx, v.FS, roots)
}

// conflictError is what validateNoCoexistence reports for two config files the walk found in one directory.
type conflictError struct {
	error
}

// conflict returns a conflictError when the directory of path, the config file of found, holds another config
// file that validateNoCoexistence rejects beside it.
func conflict(
	fsys vfs.FS,
	phase *FilesystemPhase,
	input *PhaseInput,
	found DiscoveryResult,
	path string,
	filenames []string,
) error {
	dir, base := filepath.Split(path)

	for _, filename := range filenames {
		if filename == base {
			continue
		}

		sibling := filepath.Join(dir, filename)
		if !vfs.IsFile(fsys, sibling) {
			continue
		}

		other := phase.processFile(input, sibling, filenames)
		if other == nil || other.Status != found.Status {
			continue
		}

		if err := validateNoCoexistence([]DiscoveryResult{found, *other}); err != nil {
			return conflictError{err}
		}
	}

	return nil
}

// findInWorktrees runs the worktree phase, reports each component it finds on events, and queues the ones that
// need a parse on work.
func (p *pipeline) findInWorktrees(ctx context.Context, l log.Logger, v *venv.Venv) error {
	results, err := p.discovery.runWorktreeWalk(ctx, l, v, p.store, p.opts)
	if err != nil {
		return err
	}

	for _, result := range slices.Concat(results.Discovered, results.Candidates) {
		if err := p.report(ctx, v.FS, result, stageInWorktree); err != nil {
			return err
		}
	}

	return nil
}

// report sends result on events as what a finder found at stage, and queues it on work when a worker has to parse
// it or read its dependencies.
//
// Returns the error of ctx when it is done before a send completes.
func (p *pipeline) report(ctx context.Context, fsys vfs.FS, result DiscoveryResult, stage pipelineStage) error {
	select {
	case p.events <- pipelineEvent{result: result, stage: stage}:
	case <-ctx.Done():
		return ctx.Err()
	}

	if !p.needsParse(result) && !p.readsDependencies(fsys, result) {
		return nil
	}

	select {
	case p.work <- result:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// needsParse reports whether a finder queues result for a parse: a candidate whose filters read its config, and
// every component found when an option asks for what a parse fills in.
func (p *pipeline) needsParse(result DiscoveryResult) bool {
	if !p.parseRequested {
		return false
	}

	if result.Status == filter.StatusCandidate {
		return result.Reason == filter.CandidacyReasonRequiresParse
	}

	return p.discovery.parsesDiscovered()
}

// parsesDiscovered reports whether an option asks for what a parse fills in on every component found.
func (d *Discovery) parsesDiscovered() bool {
	return d.readFiles || d.parseExclude || d.parseIncludes
}

// readsDependencies reports whether a worker reads the dependencies of result's component. It reads those of the
// target of a graph filter, and when a filter selects dependents, those of a component ready for filtering and of a
// candidate one of that filter's traversals can reach.
func (p *pipeline) readsDependencies(fsys vfs.FS, result DiscoveryResult) bool {
	switch result.Status {
	case filter.StatusReadyForFilter:
		return p.findsDependents
	case filter.StatusCandidate:
		if result.Reason == filter.CandidacyReasonGraphTarget {
			return true
		}

		return p.findsDependents && !outsideDependentBoundaries(fsys, p.dependentBoundaries, result)
	case filter.StatusExcluded:
	}

	return false
}

// parse works through the components on work until it closes and reports each on events. It parses a component
// that needs it, then reads the dependencies of one discovery needs them from. Once ctx is done, it drains work
// without reading anything.
func (p *pipeline) parse(ctx context.Context, l log.Logger, v *venv.Venv) {
	phase := NewParsePhase(p.discovery.numWorkers)

	for candidate := range p.work {
		if ctx.Err() != nil {
			continue
		}

		event := pipelineEvent{result: candidate, stage: stageRelated}

		if p.needsParse(candidate) {
			event.stage = stageParsed

			parsed, err := phase.parseAndReclassify(ctx, l, v, p.store, p.opts, p.discovery, candidate)
			if err != nil {
				event.err = err
			}

			if parsed != nil {
				event.result = *parsed
			}
		}

		if event.err == nil && p.readsDependencies(v.FS, event.result) {
			declared := readDependencyPaths(
				contextWithParsePhase(ctx, parsePhaseTagDependencyGraph),
				l,
				v,
				p.store,
				p.opts,
				p.discovery,
				event.result.Component,
			)
			event.declared = &declared
		}

		select {
		case p.events <- event:
		case <-ctx.Done():
		}
	}
}

// collect receives every event until events closes.
func (p *pipeline) collect() *collected {
	c := &collected{
		parsed:         make(map[component.Component]DiscoveryResult),
		parseErrs:      make(map[component.Component]error),
		declared:       make(map[component.Component]dependencyPaths),
		parseRequested: p.parseRequested,
	}

	for event := range p.events {
		c.add(&event)
	}

	return c
}

// foundResults is the components one finder classified without a parse, in the order it reported them.
type foundResults struct {
	// discovered is the components ready for filtering.
	discovered []DiscoveryResult
	// candidates is the components left for a parse or the graph phase.
	candidates []DiscoveryResult
}

// add records result.
func (f *foundResults) add(result DiscoveryResult) {
	if result.Status == filter.StatusCandidate {
		f.candidates = append(f.candidates, result)

		return
	}

	f.discovered = append(f.discovered, result)
}

// collected is what the collector holds once the pipeline has drained.
type collected struct {
	// parsed maps each component that parsed to its classification after the parse.
	parsed map[component.Component]DiscoveryResult
	// parseErrs maps each component that failed to parse to the failure.
	parseErrs map[component.Component]error
	// declared maps each component a worker read the dependencies of to those dependencies.
	declared map[component.Component]dependencyPaths
	// walked is what the walk found.
	walked foundResults
	// inWorktrees is what the worktree phase found.
	inWorktrees foundResults
	// parseRequested reports whether an option or a filter asked for a parse.
	parseRequested bool
}

// add records event.
func (c *collected) add(event *pipelineEvent) {
	if event.declared != nil {
		c.declared[event.result.Component] = *event.declared
	}

	switch event.stage {
	case stageRelated:
	case stageParsed:
		if event.err != nil {
			c.parseErrs[event.result.Component] = event.err

			return
		}

		c.parsed[event.result.Component] = event.result
	case stageInWorktree:
		c.inWorktrees.add(event.result)
	case stageWalked:
		c.walked.add(event.result)
	}
}

// results returns the discovered components and candidates the filesystem and parse phases would hand the graph
// phase for what one finder found, and the parse failures among them. The parse of a component that lost to another
// at its path is left out.
//
// Returns a [CoexistenceError] when a directory holds both a unit and a stack, and a [config.AmbiguousConfigError]
// when it holds both terragrunt.hcl and terragrunt.hcl.json.
func (c *collected) results(d *Discovery, found foundResults) (*PhaseResults, []error, error) {
	if err := validateNoCoexistence(found.discovered); err != nil {
		return nil, nil, err
	}

	if err := validateNoCoexistence(found.candidates); err != nil {
		return nil, nil, err
	}

	discovered := deduplicateResults(found.discovered)
	candidates := deduplicateResults(found.candidates)

	if !c.parseRequested {
		return &PhaseResults{Discovered: discovered, Candidates: candidates}, nil, nil
	}

	results := &PhaseResults{Discovered: slices.Clone(discovered)}

	var parseErrs []error

	place := func(walked DiscoveryResult) {
		if err, failed := c.parseErrs[walked.Component]; failed {
			parseErrs = append(parseErrs, err)

			return
		}

		parsed, ok := c.parsed[walked.Component]
		if !ok {
			return
		}

		switch parsed.Status {
		case filter.StatusReadyForFilter:
			results.Discovered = append(results.Discovered, parsed)
		case filter.StatusCandidate:
			results.Candidates = append(results.Candidates, parsed)
		case filter.StatusExcluded:
		}
	}

	for _, candidate := range candidates {
		if candidate.Reason != filter.CandidacyReasonRequiresParse {
			results.Candidates = append(results.Candidates, candidate)

			continue
		}

		place(candidate)
	}

	if d.parsesDiscovered() {
		for _, found := range discovered {
			place(found)
		}
	}

	results.Discovered = deduplicateResults(results.Discovered)

	return results, parseErrs, nil
}
