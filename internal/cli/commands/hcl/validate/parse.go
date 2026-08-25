package validate

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
)

// DiagnosticsCollector gathers the diagnostics a parse reports, for a caller
// that answers with them rather than letting the parser's default diagnostics
// writer print them.
type DiagnosticsCollector struct {
	diags diagnostic.Diagnostics
}

// Diagnostics returns what the parses collected so far.
func (c *DiagnosticsCollector) Diagnostics() diagnostic.Diagnostics {
	return c.diags
}

// Option returns the [hclparse.Option] that feeds c.
func (c *DiagnosticsCollector) Option() hclparse.Option {
	return hclparse.WithDiagnosticsHandler(c.collect)
}

// collect keeps the diagnostics raised for the file being parsed. It reports
// none of them back to the parser, so the parse carries on past what it
// collected and the caller decides what a diagnostic means.
func (c *DiagnosticsCollector) collect(
	file *hcl.File,
	hclDiags hcl.Diagnostics,
) (hcl.Diagnostics, error) {
	for _, hclDiag := range hclDiags {
		if raisedElsewhere(file, hclDiag) {
			continue
		}

		c.add(diagnostic.NewDiagnostic(file, hclDiag))
	}

	return nil, nil
}

// add keeps diag unless an equivalent one is already collected.
func (c *DiagnosticsCollector) add(diag *diagnostic.Diagnostic) {
	if c.has(diag) {
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

// raisedElsewhere reports whether diag points at a file other than the one
// being parsed. A parse raises what its dependencies and includes report too,
// and those belong to whichever file declared them.
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
	Collector *DiagnosticsCollector
	Options   ParserOptions
}

// Unit parses the unit configuration in unitDir.
func (p ComponentParser) Unit(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	unitDir string,
) error {
	parseOpts := opts.Clone()
	parseOpts.WorkingDir = unitDir
	parseOpts.TerragruntConfigPath = filepath.Join(unitDir, unitConfigFilename(opts))
	parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

	_, pctx := configbridge.NewParsingContext(ctx, l, v, parseOpts)

	_, err := config.ReadTerragruntConfig(ctx, l, pctx, p.parseOptions(pctx.ParserOptions))

	return err
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
	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)

	parseOpts := opts.Clone()
	parseOpts.WorkingDir = stackDir
	parseOpts.TerragruntConfigPath = stackFilePath

	sctx, parser := configbridge.NewParsingContext(ctx, l, v, parseOpts)

	var parseErrs []error

	values, err := config.ReadValues(sctx, parser, l, stackDir)
	if err != nil {
		parseErrs = append(parseErrs, err)
	}

	parser = parser.WithParseOption(p.parseOptions(parser.ParserOptions))
	if values != nil {
		parser = parser.WithValues(values)
	}

	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(v.FS, stackFilePath)
	if err != nil {
		return append(parseErrs, err)
	}

	stackCfg, err := config.ParseStackConfig(sctx, l, parser, file, values)
	if err != nil {
		return append(parseErrs, err)
	}

	// The lenient stack decode above leaves autoinclude blocks unvalidated, so
	// run the strict autoinclude parse `stack generate` uses. It no-ops unless
	// the stack-dependencies experiment is enabled and the config declares
	// autoinclude.
	autoIncludeErr := config.ValidateStackAutoIncludes(
		sctx, l, parser, stackFilePath, stackCfg, values,
	)
	if autoIncludeErr != nil {
		parseErrs = append(parseErrs, autoIncludeErr)
	}

	return parseErrs
}

func (p ComponentParser) parseOptions(defaults []hclparse.Option) []hclparse.Option {
	if p.Options != CollectorAndDefaults {
		return []hclparse.Option{p.Collector.Option()}
	}

	// A fresh slice, since appending to the parsing context's own would write
	// into whatever else shares its backing array.
	opts := make([]hclparse.Option, 0, len(defaults)+1)
	opts = append(opts, defaults...)

	return append(opts, p.Collector.Option())
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
