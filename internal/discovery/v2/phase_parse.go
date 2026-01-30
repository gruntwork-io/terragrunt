package v2

import (
	"context"
	"io"
	"path/filepath"

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
	if numWorkers <= 0 {
		numWorkers = defaultDiscoveryWorkers
	}

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
func (p *ParsePhase) Run(ctx context.Context, input *PhaseInput) PhaseOutput {
	discovered := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	candidates := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	errors := make(chan error, p.numWorkers)
	done := make(chan struct{})

	go func() {
		defer close(discovered)
		defer close(candidates)
		defer close(errors)
		defer close(done)

		p.runParsing(ctx, input, discovered, candidates, errors)
	}()

	return PhaseOutput{
		Discovered: discovered,
		Candidates: candidates,
		Done:       done,
		Errors:     errors,
	}
}

// runParsing performs the actual parsing.
func (p *ParsePhase) runParsing(
	ctx context.Context,
	input *PhaseInput,
	discovered chan<- DiscoveryResult,
	candidates chan<- DiscoveryResult,
	errors chan<- error,
) {
	discovery := input.Discovery

	componentsToParse := make([]DiscoveryResult, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.Reason == CandidacyReasonRequiresParse {
			componentsToParse = append(componentsToParse, candidate)

			continue
		}

		select {
		case candidates <- candidate:
		case <-ctx.Done():
			return
		}
	}

	// When readFiles or parseExclude is enabled, also parse discovered components
	// to populate the Reading field or Exclude configuration even without filters
	if discovery.readFiles || discovery.parseExclude {
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
		return
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	for _, candidate := range componentsToParse {
		g.Go(func() error {
			result, err := p.parseAndReclassify(ctx, input.Logger, input.Opts, discovery, candidate, input.Classifier)
			if err != nil {
				select {
				case errors <- err:
				default:
				}
				// Return nil to continue processing other components
				// Error is sent to channel for collection
				return nil //nolint:nilerr
			}

			if result == nil {
				return nil
			}

			switch result.Status {
			case StatusDiscovered:
				select {
				case discovered <- *result:
				case <-ctx.Done():
					return ctx.Err()
				}
			case StatusCandidate:
				select {
				case candidates <- *result:
				case <-ctx.Done():
					return ctx.Err()
				}
			case StatusExcluded:
				// Excluded components are not sent to any channel
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		select {
		case errors <- err:
		default:
		}
	}
}

// parseAndReclassify parses a component and reclassifies it based on filter evaluation.
func (p *ParsePhase) parseAndReclassify(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovery *Discovery,
	candidate DiscoveryResult,
	classifier *filter.Classifier,
) (*DiscoveryResult, error) {
	c := candidate.Component

	_, ok := c.(*component.Unit)
	if !ok {
		return &candidate, nil
	}

	if err := parseComponent(c, ctx, l, opts, discovery.suppressParseErrors, discovery.parserOptions); err != nil {
		if discovery.suppressParseErrors {
			l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)
			return nil, nil
		}

		return nil, err
	}

	if classifier != nil {
		for _, expr := range classifier.ParseExpressions() {
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
		status, reason, graphIdx := classifier.Classify(c, classCtx)

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
	c component.Component,
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	suppressParseErrors bool,
	parserOptions []hclparse.Option,
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
	).WithSkipOutputsResolution()

	if len(parserOptions) > 0 {
		parsingCtx = parsingCtx.WithParseOption(parserOptions)
	}

	if suppressParseErrors {
		opts := parsingCtx.ParserOptions
		opts = append(opts, hclparse.WithDiagnosticsHandler(func(
			file *hcl.File,
			hclDiags hcl.Diagnostics,
		) (hcl.Diagnostics, error) {
			l.Debugf("Suppressed parsing errors %v", hclDiags)
			return nil, nil
		}))
		parsingCtx = parsingCtx.WithParseOption(opts)
	}

	cfg, err := config.PartialParseConfigFile(ctx, parsingCtx, l, parseOpts.TerragruntConfigPath, nil)
	if err != nil {
		if suppressParseErrors {
			var notFoundErr config.TerragruntConfigNotFoundError
			if errors.As(err, &notFoundErr) {
				l.Debugf("Skipping missing config during discovery: %s", parseOpts.TerragruntConfigPath)
				return nil
			}
		}

		if !suppressParseErrors || cfg == nil {
			return errors.New(err)
		}

		l.Debugf("Suppressing parse error for %s: %s", parseOpts.TerragruntConfigPath, err)
	}

	if unit, ok := c.(*component.Unit); ok {
		unit.StoreConfig(cfg)
	}

	if parsingCtx.FilesRead != nil {
		c.SetReading(*parsingCtx.FilesRead...)
	}

	return nil
}
