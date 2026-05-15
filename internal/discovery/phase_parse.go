package discovery

import (
	"context"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
	"golang.org/x/sync/errgroup"
)

// Phase tag values for parseComponent call-site identification in telemetry.
const (
	parsePhaseTagParse             = "parse"
	parsePhaseTagGraphDependencies = "graph_dependencies"
	parsePhaseTagGraphDependents   = "graph_dependents"
	parsePhaseTagDependencyGraph   = "dependency_graph"
	parsePhaseTagRelationship      = "relationship"
	parsePhaseTagUnknown           = "unknown"
)

// parseReason tags a contributing cause that the parse phase ran.
// Multiple causes can apply to one discovery; values are surfaced on the
// discovery_phase_parse telemetry span so the activation cause is auditable.
type parseReason string

const (
	parseReasonExplicit                parseReason = "explicit"
	parseReasonParseExclude            parseReason = "parse-exclude"
	parseReasonParseIncludes           parseReason = "parse-includes"
	parseReasonReadFiles               parseReason = "read-files"
	parseReasonFiltersRequireParse     parseReason = "filters-require-parse"
	parseReasonClassifierRequiresParse parseReason = "classifier-parse-required"
)

// joinParseReasons sorts to keep telemetry values stable across runs.
func joinParseReasons(reasons []parseReason) string {
	sorted := slices.Clone(reasons)
	slices.Sort(sorted)

	parts := make([]string, len(sorted))
	for i, r := range sorted {
		parts[i] = string(r)
	}

	return strings.Join(parts, ",")
}

type parsePhaseKey struct{}

type parseDepthKey struct{}

// contextWithParsePhase returns ctx tagged with the calling phase for parseComponent telemetry.
func contextWithParsePhase(ctx context.Context, phase string) context.Context {
	return context.WithValue(ctx, parsePhaseKey{}, phase)
}

// parsePhaseFromContext returns the phase tag attached to ctx, or "unknown".
func parsePhaseFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(parsePhaseKey{}).(string); ok {
		return v
	}

	return parsePhaseTagUnknown
}

// contextWithIncrementedParseDepth returns ctx with the parse-recursion depth bumped by one.
// Use this at every recursion site that ultimately calls ensureParsed (graph traversal,
// dependents upstream walk, relationship traversal) so children observe a deeper depth.
func contextWithIncrementedParseDepth(ctx context.Context) context.Context {
	return context.WithValue(ctx, parseDepthKey{}, parseDepthFromContext(ctx)+1)
}

// parseDepthFromContext returns the recursion depth attached to ctx, or 0.
func parseDepthFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(parseDepthKey{}).(int); ok {
		return v
	}

	return 0
}

// ensureParsed parses c if its config is not already cached on the underlying Unit.
// Callers must set the phase tag via contextWithParsePhase before calling, and
// increment depth via contextWithIncrementedParseDepth at recursion sites.
func ensureParsed(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c component.Component,
	opts *options.TerragruntOptions,
	discovery *Discovery,
) error {
	unit, ok := c.(*component.Unit)
	if !ok {
		return nil
	}

	if unit.Config() != nil {
		// Cache hits are emitted as a counter rather than a span. A graph or
		// relationship traversal can re-visit the same component many times, and
		// a span per visit drowns out the parse misses that are the actual cost.
		telemetry.TelemeterFromContext(ctx).Count(ctx, "discovery_parse_cache_hit", 1)
		l.Debugf(
			"Discovery: parse cache hit for %s (phase=%s, depth=%d)",
			c.Path(), parsePhaseFromContext(ctx), parseDepthFromContext(ctx),
		)

		return nil
	}

	return parseComponent(ctx, l, v, c, opts, discovery)
}

// ParsePhase parses HCL configurations for filter evaluation.
type ParsePhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
}

// NewParsePhase creates a new ParsePhase.
func NewParsePhase(numWorkers int) *ParsePhase {
	numWorkers = max(numWorkers, defaultDiscoveryWorkers)

	return &ParsePhase{
		numWorkers: numWorkers,
	}
}

// Name returns the human-readable name of the phase.
func (p *ParsePhase) Name() string {
	return "parse"
}

// Kind returns the PhaseKind identifier.
func (p *ParsePhase) Kind() PhaseKind {
	return PhaseParse
}

// Run executes the parse phase.
func (p *ParsePhase) Run(ctx context.Context, l log.Logger, v *venv.Venv, input *PhaseInput) (*PhaseResults, error) {
	results := NewPhaseResults()
	discovery := input.Discovery

	componentsToParse := make([]DiscoveryResult, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.Reason == filter.CandidacyReasonRequiresParse {
			componentsToParse = append(componentsToParse, candidate)

			continue
		}

		results.AddCandidate(candidate)
	}

	// When readFiles, parseExclude, or parseIncludes is enabled, also parse discovered components
	// to populate the Reading field, Exclude configuration, or ProcessedIncludes even without filters
	if discovery.readFiles || discovery.parseExclude || discovery.parseIncludes {
		for _, c := range input.Components {
			componentsToParse = append(componentsToParse, DiscoveryResult{
				Component: c,
				Status:    filter.StatusReadyForFilter,
				Reason:    filter.CandidacyReasonNone,
				Phase:     PhaseParse,
			})
		}
	}

	if len(componentsToParse) == 0 {
		return results, nil
	}

	var (
		errs  []error
		errMu sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, candidate := range componentsToParse {
		g.Go(func() error {
			result, err := p.parseAndReclassify(ctx, l, v, input.Opts, discovery, candidate)
			if err != nil {
				errMu.Lock()

				errs = append(errs, err)

				errMu.Unlock()
				// Return nil to continue processing other components
				return nil //nolint:nilerr
			}

			if result == nil {
				return nil
			}

			switch result.Status {
			case filter.StatusReadyForFilter:
				results.AddDiscovered(*result)
			case filter.StatusCandidate:
				results.AddCandidate(*result)
			case filter.StatusExcluded:
				// Excluded components are not added
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return results, errors.Join(errs...)
	}

	return results, nil
}

// parseAndReclassify parses a component and reclassifies it based on filter evaluation.
func (p *ParsePhase) parseAndReclassify(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	discovery *Discovery,
	candidate DiscoveryResult,
) (*DiscoveryResult, error) {
	c := candidate.Component
	ctx = contextWithParsePhase(ctx, parsePhaseTagParse)

	if err := parseComponent(ctx, l, v, c, opts, discovery); err != nil {
		if discovery.suppressParseErrors {
			l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)

			return &DiscoveryResult{
				Component: c,
				Status:    filter.StatusExcluded,
				Reason:    filter.CandidacyReasonNone,
				Phase:     PhaseParse,
			}, nil
		}

		return nil, err
	}

	if discovery.classifier != nil {
		for _, expr := range discovery.classifier.ParseExpressions() {
			matched, err := filter.Evaluate(l, expr, component.Components{c})
			if err != nil {
				l.Debugf("Error evaluating parse expression for %s: %v", c.Path(), err)
				continue
			}

			if len(matched) > 0 {
				return &DiscoveryResult{
					Component: c,
					Status:    filter.StatusReadyForFilter,
					Reason:    filter.CandidacyReasonNone,
					Phase:     PhaseParse,
				}, nil
			}
		}

		classCtx := filter.ClassificationContext{ParseDataAvailable: true}
		status, reason, graphIdx := discovery.classifier.Classify(c, classCtx)

		return &DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			Phase:                PhaseParse,
			GraphExpressionIndex: graphIdx,
		}, nil
	}

	return &DiscoveryResult{
		Component: c,
		Status:    candidate.Status,
		Reason:    candidate.Reason,
		Phase:     PhaseParse,
	}, nil
}

// parseComponent parses a Terragrunt configuration.
func parseComponent(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c component.Component,
	opts *options.TerragruntOptions,
	discovery *Discovery,
) error {
	phase := parsePhaseFromContext(ctx)
	depth := parseDepthFromContext(ctx)

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "discovery_parse_component", map[string]any{
		"path":      c.Path(),
		"phase":     phase,
		"depth":     depth,
		"cache_hit": false,
	}, func(ctx context.Context) error {
		l.Debugf("Discovery: parsing %s (phase=%s, depth=%d)", c.Path(), phase, depth)

		parseOpts := opts.Clone()

		componentPath := c.Path()
		workingDir := componentPath

		if util.FileExists(componentPath) && !util.IsDir(componentPath) {
			workingDir = filepath.Dir(componentPath)
		}

		configFilename := config.DefaultTerragruntConfigPath

		switch c.(type) {
		case *component.Stack:
			configFilename = config.DefaultStackFile
		default:
			if unit, ok := c.(*component.Unit); ok && unit.ConfigFile() != "" {
				configFilename = unit.ConfigFile()
				break
			}

			if opts.TerragruntConfigPath != "" && !util.IsDir(opts.TerragruntConfigPath) {
				configFilename = filepath.Base(opts.TerragruntConfigPath)
			}
		}

		parseOpts.WorkingDir = workingDir
		parseOpts.SkipOutput = true
		parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, configFilename)
		parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

		// Clone v.Env so concurrent parseComponent goroutines launched by
		// ParsePhase and RelationshipPhase don't race on the shared map when
		// ObtainCredsForParsing writes auth-provider-cmd output into it.
		parseV := *v
		parseV.Env = maps.Clone(v.Env)
		parseV.Writers = parseV.Writers.WithWriter(io.Discard).WithErrWriter(io.Discard)

		shellOpts := configbridge.ShellRunOptsFromOpts(parseV.Env, parseOpts)

		_, err := creds.ObtainCredsForParsing(ctx, l, &parseV, parseOpts.AuthProviderCmd, parseV.Env, shellOpts)
		if err != nil {
			return errors.Errorf("obtaining auth provider credentials for %s: %w", parseOpts.TerragruntConfigPath, err)
		}

		ctx, parsingCtx := configbridge.NewParsingContext(ctx, l, parseOpts)
		parsingCtx.Venv = &parseV
		parsingCtx = parsingCtx.WithDecodeList(
			config.TerraformSource,
			config.DependenciesBlock,
			config.DependencyBlock,
			config.TerragruntFlags,
			config.FeatureFlagsBlock,
			config.ExcludeBlock,
			config.ErrorsBlock,
			config.RemoteStateBlock,
			config.TerragruntVersionConstraints,
		).WithSkipOutputsResolution()

		if len(discovery.parserOptions) > 0 {
			parsingCtx = parsingCtx.WithParseOption(discovery.parserOptions)
		}

		if discovery.suppressParseErrors {
			parserOpts := parsingCtx.ParserOptions
			parserOpts = append(parserOpts, hclparse.WithDiagnosticsHandler(func(
				file *hcl.File,
				hclDiags hcl.Diagnostics,
			) (hcl.Diagnostics, error) {
				l.Debugf("Suppressed parsing errors %v", hclDiags)
				return nil, nil
			}))
			parsingCtx = parsingCtx.WithParseOption(parserOpts)
		}

		cfg, err := config.PartialParseConfigFile(ctx, parsingCtx, l, parseOpts.TerragruntConfigPath, nil)
		if err != nil {
			if discovery.suppressParseErrors {
				var notFoundErr config.TerragruntConfigNotFoundError
				if errors.As(err, &notFoundErr) {
					l.Debugf("Skipping missing config during discovery: %s", parseOpts.TerragruntConfigPath)
					return nil
				}
			}

			if !discovery.suppressParseErrors || cfg == nil {
				return err
			}

			l.Debugf("Suppressing parse error for %s: %s", parseOpts.TerragruntConfigPath, err)
		}

		if unit, ok := c.(*component.Unit); ok {
			unit.StoreConfig(cfg)
		}

		if parsingCtx.FilesRead != nil {
			readFiles := sanitizeReadFiles(*parsingCtx.FilesRead)
			c.SetReading(readFiles...)
		}

		return nil
	})
}
