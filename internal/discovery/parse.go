package discovery

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

// parseReason names one reason discovery parses components. Several can apply
// to one discovery, and the discovery_pipeline telemetry span records each.
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

// contextWithParseDepth returns ctx with the parse-recursion depth set to depth.
func contextWithParseDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, parseDepthKey{}, depth)
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
	s *config.Store,
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

	return unit.GuardConfigParse(func() error {
		return parseComponent(ctx, l, v, s, c, opts, discovery)
	})
}

// parseAndReclassify parses a component and reclassifies it based on filter evaluation.
func (d *Discovery) parseAndReclassify(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
	candidate DiscoveryResult,
) (*DiscoveryResult, error) {
	c := candidate.Component
	ctx = contextWithParsePhase(ctx, parsePhaseTagParse)

	if err := parseComponent(ctx, l, v, s, c, opts, d); err != nil {
		if d.suppressParseErrors {
			l.Debugf("Suppressed parse error for %s: %v", c.Path(), err)

			return &DiscoveryResult{
				Component: c,
				Status:    filter.StatusExcluded,
				Reason:    filter.CandidacyReasonNone,
			}, nil
		}

		return nil, err
	}

	if d.classifier != nil {
		for _, expr := range d.classifier.ParseExpressions() {
			matched, err := filter.Evaluate(l, d.evaluationContext(), expr, component.Components{c})
			if err != nil {
				l.Debugf("Error evaluating parse expression for %s: %v", c.Path(), err)
				continue
			}

			if len(matched) > 0 {
				return &DiscoveryResult{
					Component: c,
					Status:    filter.StatusReadyForFilter,
					Reason:    filter.CandidacyReasonNone,
				}, nil
			}
		}

		classCtx := filter.ClassificationContext{ParseDataAvailable: true}
		status, reason, graphIdx := d.classifier.Classify(c, classCtx)

		return &DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			GraphExpressionIndex: graphIdx,
		}, nil
	}

	return &DiscoveryResult{
		Component: c,
		Status:    candidate.Status,
		Reason:    candidate.Reason,
	}, nil
}

// parseComponent parses a Terragrunt configuration.
func parseComponent(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	c component.Component,
	opts *options.TerragruntOptions,
	discovery *Discovery,
) error {
	phase := parsePhaseFromContext(ctx)
	depth := parseDepthFromContext(ctx)

	return telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "discovery_parse_component", map[string]any{
			"path":      c.Path(),
			"phase":     phase,
			"depth":     depth,
			"cache_hit": false,
		}, func(ctx context.Context, l log.Logger) error {
			l.Debugf("Discovery: parsing %s (phase=%s, depth=%d)", c.Path(), phase, depth)

			parseOpts := opts.Clone()

			componentPath := c.Path()
			workingDir := componentPath

			pathIsFile := vfs.IsFile(v.FS, componentPath)
			if pathIsFile {
				workingDir = filepath.Dir(componentPath)

				if unit, ok := c.(*component.Unit); ok {
					unit.SetConfigDir(workingDir)
				}
			}

			configFilename := config.DefaultTerragruntConfigPath

			switch c.(type) {
			case *component.Stack:
				configFilename = config.DefaultStackFile
			default:
				if pathIsFile {
					configFilename = filepath.Base(componentPath)
					break
				}

				if unit, ok := c.(*component.Unit); ok && unit.ConfigFile() != "" {
					configFilename = unit.ConfigFile()
					break
				}

				if opts.TerragruntConfigPath != "" && !vfs.IsDir(v.FS, opts.TerragruntConfigPath) {
					configFilename = filepath.Base(opts.TerragruntConfigPath)
				}
			}

			parseOpts.WorkingDir = workingDir
			parseOpts.SkipOutput = true
			parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, configFilename)
			parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

			// Clone v.Env so concurrent parseComponent calls don't race on the
			// shared map when ObtainCredsForParsing writes auth-provider-cmd
			// output into it.
			parseV := v.WithEnvCloned().WithWriter(io.Discard).WithErrWriter(io.Discard)

			shellOpts := configbridge.ShellRunOptsFromOpts(v.Exec, v.Env, parseOpts)

			if parseOpts.DiscoveryAuthProviderCmd {
				if _, err := creds.ObtainCredsForParsing(
					ctx, l, parseV, parseOpts.AuthProviderCmd, shellOpts,
				); err != nil {
					return fmt.Errorf(
						"obtaining auth provider credentials for %s: %w",
						parseOpts.TerragruntConfigPath,
						err,
					)
				}
			}

			parsingCtx := configbridge.NewParsingContext(parseOpts)
			parsingCtx = parsingCtx.WithDecodeList(
				config.TerraformSource,
				config.DependenciesBlock,
				config.DependencyBlock,
				config.TerragruntFlags,
				config.FeatureFlagsBlock,
				config.ExcludeBlock,
				config.ErrorsBlock,
				config.TerragruntVersionConstraints,
				config.EngineBlock,
			).WithSkipOutputsResolution()

			if discovery.trackReads {
				parsingCtx = parsingCtx.WithFileReadTracking()
			}

			if discovery.suppressParseErrors {
				parsingCtx = parsingCtx.Clone()
				parsingCtx.Parser.IgnoreDiagnostics = true
			}

			cfg, err := s.ResolveUnitQueueConfig(
				ctx,
				l,
				parseV,
				parsingCtx,
				parseOpts.TerragruntConfigPath,
			)
			if err != nil {
				if discovery.suppressParseErrors {
					if errors.As(err, new(config.TerragruntConfigNotFoundError)) {
						l.Debugf(
							"Skipping missing config during discovery: %s",
							parseOpts.TerragruntConfigPath,
						)

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

			if !parsingCtx.FilesRead.Tracking() {
				return nil
			}

			if _, ok := c.(*component.Stack); ok {
				autoIncludes := trackStackComponentReads(ctx, l, parseV, s, parsingCtx, parseOpts.TerragruntConfigPath)
				discovery.recordAutoIncludeInputsReads(l, parseOpts.TerragruntConfigPath, autoIncludes)
			} else {
				discovery.recordUnitInputsReads(l, v.FS, parseOpts.TerragruntConfigPath, cfg)
			}

			c.SetReading(sanitizeReadFiles(parsingCtx.FilesRead.Paths())...)

			return nil
		})
}

// trackStackComponentReads records in pctx.FilesRead the files read by the unit and
// stack blocks of the stack file at stackFilePath, and by the config_path and expansion
// of each dependency in their autoinclude blocks, and returns the resolved autoinclude
// blocks. Stack generation evaluates all of these, but the partial parse evaluates only
// locals.
//
// It is best-effort and never fails discovery. Diagnostics are logged at debug level
// and dropped, and a parse that fails keeps the reads recorded before the failure,
// since a catalog stack can reference values.* that only the parent stack generating
// it supplies.
func trackStackComponentReads(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	pctx *config.ParsingContext,
	stackFilePath string,
) map[string]*inthclparse.AutoIncludeResolved {
	pctx = pctx.Clone()
	pctx.Parser.IgnoreDiagnostics = true

	values, err := config.ReadValues(ctx, l, v, s, pctx, filepath.Dir(stackFilePath))
	if err != nil {
		l.Debugf("Discovery: skipping read tracking for %s: %v", stackFilePath, err)

		return nil
	}

	stackCfg, err := config.ReadStackConfigFile(ctx, l, v, s, pctx, stackFilePath, values)
	if err != nil {
		l.Debugf("Discovery: read tracking for %s stopped early: %v", stackFilePath, err)

		return nil
	}

	autoIncludes, err := config.StackAutoIncludes(ctx, l, v, s, pctx, stackFilePath, stackCfg, values)
	if err != nil {
		l.Debugf("Discovery: read tracking for %s stopped early: %v", stackFilePath, err)
	}

	return autoIncludes
}
