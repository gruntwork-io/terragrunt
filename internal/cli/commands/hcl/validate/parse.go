package validate

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
)

// DiagnosticsCollector gathers the diagnostics a parse reports, for a caller
// that answers with them rather than letting the parser's default diagnostics
// writer print them.
type DiagnosticsCollector struct {
	// missingDependencyUnits has the directory of each unit that has a diagnostic for a missing dependency.
	missingDependencyUnits map[string]struct{}
	// owners maps each diagnostic to the directory of the component whose validation collected it.
	owners map[*diagnostic.Diagnostic]string
	// diags are the diagnostics collected so far, without repeats.
	diags diagnostic.Diagnostics
}

// Diagnostics returns what the parses collected so far.
func (c *DiagnosticsCollector) Diagnostics() diagnostic.Diagnostics {
	return c.diags
}

// owner returns the directory of the component whose validation collected diag.
func (c *DiagnosticsCollector) owner(diag *diagnostic.Diagnostic) string {
	return c.owners[diag]
}

// attribute records dir as the owner of every diagnostic collected since the collector held from
// of them. A repeat that [DiagnosticsCollector.add] dropped keeps the owner of the first.
func (c *DiagnosticsCollector) attribute(from int, dir string) {
	if c.owners == nil {
		c.owners = make(map[*diagnostic.Diagnostic]string)
	}

	for _, diag := range c.diags[from:] {
		c.owners[diag] = dir
	}
}

// collect keeps the diagnostics raised for the file being parsed. It reports
// none of them back to the parser, so the parse carries on past what it
// collected and the caller decides what a diagnostic means.
func (c *DiagnosticsCollector) collect(
	file *hcl.File,
	hclDiags hcl.Diagnostics,
) (hcl.Diagnostics, error) {
	return nil, c.keep(file, hclDiags)
}

// keep collects the diagnostics of hclDiags that point at file, one per source range.
func (c *DiagnosticsCollector) keep(file *hcl.File, hclDiags hcl.Diagnostics) error {
	return c.keepWith(file, hclDiags, c.add)
}

// keepDistinct collects the diagnostics of hclDiags that point at file, one per source range and text. Units
// reading a shared file against different dependencies raise different diagnostics at one range.
func (c *DiagnosticsCollector) keepDistinct(file *hcl.File, hclDiags hcl.Diagnostics) error {
	return c.keepWith(file, hclDiags, c.addDistinct)
}

// keepWith converts the diagnostics of hclDiags that point at file and hands each to add.
func (c *DiagnosticsCollector) keepWith(
	file *hcl.File,
	hclDiags hcl.Diagnostics,
	add func(diag *diagnostic.Diagnostic),
) error {
	for _, hclDiag := range hclDiags {
		if raisedElsewhere(file, hclDiag) {
			continue
		}

		diag, err := diagnostic.NewDiagnostic(file, hclDiag)
		if err != nil {
			return err
		}

		add(diag)
	}

	return nil
}

// add keeps diag unless one is already collected at its range.
func (c *DiagnosticsCollector) add(diag *diagnostic.Diagnostic) {
	if c.has(diag) {
		return
	}

	c.diags = append(c.diags, diag)
}

// addDistinct keeps diag unless one with its summary and detail is already collected at its range.
func (c *DiagnosticsCollector) addDistinct(diag *diagnostic.Diagnostic) {
	if c.hasDistinct(diag) {
		return
	}

	c.diags = append(c.diags, diag)
}

// has reports whether diag is already collected.
// [diagnostic.Diagnostics.Contains] matches on source range and treats a
// diagnostic without one as matching nothing, so those are compared by their
// text instead. Every unit an include points at raises the same range-less
// diagnostic, and a tree of them would otherwise repeat it per unit.
func (c *DiagnosticsCollector) has(diag *diagnostic.Diagnostic) bool {
	if diag.Range != nil {
		return c.diags.Contains(diag)
	}

	return slices.ContainsFunc(c.diags, func(have *diagnostic.Diagnostic) bool {
		return have.Range == nil && have.Summary == diag.Summary && have.Detail == diag.Detail
	})
}

// hasDistinct reports whether a diagnostic with diag's summary and detail is already collected at its range, or
// without one when diag has none.
func (c *DiagnosticsCollector) hasDistinct(diag *diagnostic.Diagnostic) bool {
	return slices.ContainsFunc(c.diags, func(have *diagnostic.Diagnostic) bool {
		return sameRange(have.Range, diag.Range) && have.Summary == diag.Summary && have.Detail == diag.Detail
	})
}

// sameRange reports whether a and b are both absent or both the same source range.
func sameRange(a, b *diagnostic.Range) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return a.String() == b.String()
}

// raisedElsewhere reports whether diag points at a file other than the one
// being parsed. A parse raises what its dependencies and includes report too,
// and those belong to whichever file declared them.
//
// A diagnostic with a Subject and no file is not dismissed here, so that
// [diagnostic.NewDiagnostic] rejects it.
func raisedElsewhere(file *hcl.File, diag *hcl.Diagnostic) bool {
	if diag.Subject == nil || file == nil {
		return false
	}

	return diag.Subject.Filename != file.Body.MissingItemRange().Filename
}

// ParserOptions says how a parse combines the collector with the options the
// parsing context installs by default.
type ParserOptions int

const (
	// CollectorOnly parses with the collector alone, which is what the hcl
	// validate command has always done. It leaves out the bare-include
	// rewrite the defaults carry, so a bare `include {}` block reads as an
	// HCL error here and parses cleanly under CollectorAndDefaults.
	CollectorOnly ParserOptions = iota
	// CollectorAndDefaults stacks the collector on the defaults, keeping the
	// bare-include strict control that lives among them.
	CollectorAndDefaults
)

// ComponentParser parses discovered components one at a time, collecting what
// each reports into Collector.
type ComponentParser struct {
	// Collector receives the diagnostics of every parse.
	Collector *DiagnosticsCollector
	// UnitConfigs are the unit configs the run parses, by path. What is wrong with one of them is reported by
	// its own parse, not at the config_path of a dependent that reads its outputs block.
	UnitConfigs map[string]struct{}
	// Options says how each parse combines the collector with the parser defaults.
	Options ParserOptions
}

// Unit parses the unit configuration in unitDir.
func (p ComponentParser) Unit(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	unitDir string,
) error {
	from := len(p.Collector.diags)
	defer p.Collector.attribute(from, unitDir)

	parseOpts := opts.Clone()
	parseOpts.WorkingDir = unitDir
	parseOpts.TerragruntConfigPath = filepath.Join(unitDir, unitConfigFilename(opts))
	parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

	pctx := configbridge.NewParsingContext(parseOpts)

	configPath := parseOpts.TerragruntConfigPath

	pctx = pctx.WithParserSettings(p.parserSettings(pctx.Parser))

	cfg, err := config.ReadTerragruntConfig(ctx, l, v, pctx)
	if err != nil {
		err = p.pointAtOutputsErrors(l, v, pctx, []string{configPath}, err)

		return p.Collector.pointAtDependencyErrors(v.FS, unitDir, []string{configPath}, err)
	}

	configPaths := dependencyConfigPaths(configPath, cfg)

	contractErr := p.pointAtOutputsErrors(
		l,
		v,
		pctx,
		configPaths,
		p.validateOutputsContracts(ctx, l, v, parseOpts, pctx, cfg),
	)

	return errors.Join(
		contractErr,
		p.Collector.pointAtDependencyErrors(
			v.FS,
			unitDir,
			configPaths,
			config.ValidateDependencyConfigPaths(v.FS, cfg, configPath),
		),
	)
}

// Stack parses the stack configuration in stackDir, returning every error the
// parse produced rather than stopping at the first.
func (p ComponentParser) Stack(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	stackDir string,
) []error {
	from := len(p.Collector.diags)
	defer p.Collector.attribute(from, stackDir)

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)

	parseOpts := opts.Clone()
	parseOpts.WorkingDir = stackDir
	parseOpts.TerragruntConfigPath = stackFilePath

	parser := configbridge.NewParsingContext(parseOpts)

	var parseErrs []error

	values, err := config.ReadValues(ctx, l, v, parser, stackDir)
	if err != nil {
		parseErrs = append(parseErrs, err)
	}

	parser = parser.WithParserSettings(p.parserSettings(parser.Parser))
	if values != nil {
		parser = parser.WithValues(values)
	}

	file, err := parser.NewParser(l, v).ParseFromFile(v.FS, stackFilePath)
	if err != nil {
		return append(parseErrs, err)
	}

	stackCfg, err := config.ParseStackConfig(ctx, l, v, parser, file, values)
	if err != nil {
		return append(parseErrs, err)
	}

	// The lenient stack decode above leaves autoinclude blocks unvalidated, so
	// run the strict autoinclude parse `stack generate` uses. It no-ops unless
	// the stack-dependencies experiment is enabled and the config declares
	// autoinclude.
	_, autoIncludeErr := config.StackAutoIncludes(
		ctx, l, v, parser, stackFilePath, stackCfg, values,
	)
	if autoIncludeErr != nil {
		parseErrs = append(parseErrs, autoIncludeErr)
	}

	return parseErrs
}

func (p ComponentParser) parserSettings(defaults config.ParserSettings) config.ParserSettings {
	if p.Options != CollectorAndDefaults {
		return config.ParserSettings{
			DiagnosticsHandler: p.Collector.collect,
			SkipDefaults:       true,
		}
	}

	defaults.DiagnosticsHandler = p.Collector.collect

	return defaults
}

// unitConfigFilename is the file name a unit parse looks for, which follows
// the configured config path so a run against terragrunt.hcl.json keeps
// looking for that name in every unit.
func unitConfigFilename(opts *options.TerragruntOptions) string {
	if opts.TerragruntConfigPath != "" {
		return filepath.Base(opts.TerragruntConfigPath)
	}

	return config.DefaultTerragruntConfigPath
}

// UnitConfigs returns the unit configs a run over components parses, by path, for [ComponentParser].
func UnitConfigs(opts *options.TerragruntOptions, components component.Components) map[string]struct{} {
	configFile := unitConfigFilename(opts)
	paths := make(map[string]struct{}, len(components))

	for _, c := range components {
		if _, ok := c.(*component.Stack); ok {
			continue
		}

		paths[filepath.Join(c.Path(), configFile)] = struct{}{}
	}

	return paths
}
