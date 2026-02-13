package discovery

import (
	"context"
	"io"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
	"golang.org/x/sync/errgroup"
)

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
func (p *ParsePhase) Run(ctx context.Context, l log.Logger, input *PhaseInput) (*PhaseResults, error) {
	results := NewPhaseResults()
	discovery := input.Discovery

	componentsToParse := make([]DiscoveryResult, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.Reason == CandidacyReasonRequiresParse {
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
				Status:    StatusDiscovered,
				Reason:    CandidacyReasonNone,
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
			result, err := p.parseAndReclassify(ctx, l, input.Opts, discovery, candidate)
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
			case StatusDiscovered:
				results.AddDiscovered(*result)
			case StatusCandidate:
				results.AddCandidate(*result)
			case StatusExcluded:
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
	opts *options.TerragruntOptions,
	discovery *Discovery,
	candidate DiscoveryResult,
) (*DiscoveryResult, error) {
	c := candidate.Component

	_, ok := c.(*component.Unit)
	if !ok {
		return &candidate, nil
	}

	if err := parseComponent(ctx, l, c, opts, discovery); err != nil {
		if discovery.suppressParseErrors {
			l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)

			return &DiscoveryResult{
				Component: c,
				Status:    StatusExcluded,
				Reason:    CandidacyReasonNone,
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
					Status:    StatusDiscovered,
					Reason:    CandidacyReasonNone,
					Phase:     PhaseParse,
				}, nil
			}
		}

		classCtx := filter.ClassificationContext{ParseDataAvailable: true}
		status, reason, graphIdx := discovery.classifier.Classify(l, c, classCtx)

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
	c component.Component,
	opts *options.TerragruntOptions,
	discovery *Discovery,
) error {
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
	parseOpts.Writer = io.Discard
	parseOpts.ErrWriter = io.Discard
	parseOpts.SkipOutput = true
	parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, configFilename)
	parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

	ctx, parsingCtx := config.NewParsingContext(ctx, l, parseOpts)
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
}
