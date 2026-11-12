package discovery

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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
// parse what work holds, classify the component again, and report it on events. When the pipeline relates
// components, the walk also queues every component ready for filtering, and a worker reads the dependencies of each
// component that is ready once it is done with it. One collector receives every event, so the results need no lock.
//
// The pipeline ends because its channels form a chain with no way back:
//
//  1. The walk ends when the tree is exhausted, it fails, or ctx is done.
//  2. The walk is the only sender on work, which closes when the walk ends.
//  3. A worker ends when work is closed and empty, so the workers end once the walk has.
//  4. The walk and the workers are the only senders on events, which closes when all of them have ended.
//  5. The collector ends when events is closed and empty.
//
// A sender gives up when ctx is done, so a full channel never holds a stage past cancellation.
type pipeline struct {
	discovery      *Discovery
	opts           *options.TerragruntOptions
	store          *rewrite.Store
	work           chan DiscoveryResult
	events         chan pipelineEvent
	parseRequested bool
	relates        bool
}

// runPipeline walks the working directory and parses the components that need it, as the filesystem and parse
// phases do one after the other. It also reads the dependencies of the components it finds when discovery relates
// components, and returns them unlinked. The relations are nil otherwise.
//
// Returns the error of the walk when it fails, and otherwise the parse errors joined. suppressParseErrors drops
// both, as it does for the phases. A [CoexistenceError] is returned either way.
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
		discovery:      d,
		opts:           opts,
		store:          s,
		work:           make(chan DiscoveryResult, workers*workQueuePerWorker),
		events:         make(chan pipelineEvent, workers*workQueuePerWorker),
		parseRequested: parseRequested,
		relates:        d.discoverRelationships,
	}

	var stages errgroup.Group

	stages.Go(func() error {
		defer close(p.work)

		return p.walk(ctx, l, v)
	})

	for range workers {
		stages.Go(func() error {
			p.parse(ctx, l, v)

			return nil
		})
	}

	drained := make(chan *collected, 1)

	go func() { drained <- p.collect() }()

	walkErr := stages.Wait()

	close(p.events)

	collected := <-drained

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(
			attribute.Int("walked", len(collected.walkedDiscovered)+len(collected.walkedCandidates)),
			attribute.Int("parsed", len(collected.parsed)+len(collected.parseErrs)),
		)
	}

	results, parseErrs, err := collected.results(d)
	if err != nil {
		return walked{}, err
	}

	found := walked{discovered: results.Discovered, candidates: results.Candidates}

	if p.relates {
		found.relations = &pipelineRelations{discovery: d, opts: opts, store: s, declared: collected.declared}
	}

	if d.suppressParseErrors {
		return found, nil
	}

	if walkErr != nil {
		return walked{}, walkErr
	}

	if len(parseErrs) > 0 {
		return walked{}, errors.Join(parseErrs...)
	}

	return found, nil
}

// walk walks the working directory, reports each component it finds on events, and queues the ones that need a
// parse on work.
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

		if !p.needsParse(*result) && !p.needsRelating(*result) {
			return nil
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

// needsParse reports whether the walk queues result for a parse: a candidate whose filters read its config, and
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

// needsRelating reports whether a worker reads the dependencies of result's component: the pipeline relates
// components, and the component is ready for filtering.
func (p *pipeline) needsRelating(result DiscoveryResult) bool {
	return p.relates && result.Status == filter.StatusReadyForFilter
}

// parse works through the components on work until it closes and reports each on events. It parses a component
// that needs it, then reads the dependencies of one that needs relating.
func (p *pipeline) parse(ctx context.Context, l log.Logger, v *venv.Venv) {
	phase := NewParsePhase(p.discovery.numWorkers)

	for candidate := range p.work {
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

		if event.err == nil && p.needsRelating(event.result) {
			declared := readDependencyPaths(ctx, l, v, p.store, p.opts, p.discovery, event.result.Component)
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

// collected is what the collector holds once the pipeline has drained.
type collected struct {
	// parsed maps each component that parsed to its classification after the parse.
	parsed map[component.Component]DiscoveryResult
	// parseErrs maps each component that failed to parse to the failure.
	parseErrs map[component.Component]error
	// declared maps each component a worker read the dependencies of to those dependencies.
	declared map[component.Component]dependencyPaths
	// walkedDiscovered is the components the walk classified as ready for filtering, in walk order.
	walkedDiscovered []DiscoveryResult
	// walkedCandidates is the components the walk classified as candidates, in walk order.
	walkedCandidates []DiscoveryResult
	// parseRequested reports whether an option or a filter asked for a parse.
	parseRequested bool
}

// add records event.
func (c *collected) add(event *pipelineEvent) {
	if event.declared != nil {
		c.declared[event.result.Component] = *event.declared
	}

	switch {
	case event.stage == stageRelated:
	case event.stage == stageParsed && event.err != nil:
		c.parseErrs[event.result.Component] = event.err
	case event.stage == stageParsed:
		c.parsed[event.result.Component] = event.result
	case event.result.Status == filter.StatusCandidate:
		c.walkedCandidates = append(c.walkedCandidates, event.result)
	default:
		c.walkedDiscovered = append(c.walkedDiscovered, event.result)
	}
}

// results returns the discovered components and candidates the filesystem and parse phases would hand the graph
// phase, and the parse failures. The parse of a component that lost to another at its path is left out.
//
// Returns a [CoexistenceError] when a directory holds both a unit and a stack.
func (c *collected) results(d *Discovery) (*PhaseResults, []error, error) {
	if err := validateNoCoexistence(c.walkedDiscovered); err != nil {
		return nil, nil, err
	}

	if err := validateNoCoexistence(c.walkedCandidates); err != nil {
		return nil, nil, err
	}

	discovered := deduplicateResults(c.walkedDiscovered)
	candidates := deduplicateResults(c.walkedCandidates)

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
