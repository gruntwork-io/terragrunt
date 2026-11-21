package config

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// assemblyParts is the parts of each file an assembly decodes, which set the errors it reports.
type assemblyParts int

const (
	// queueParts is the queue parts alone. An assembly of them reports no error for a top-level argument or
	// block the schema does not name, or for a `dependencies` path that is not a directory.
	queueParts assemblyParts = iota
	// runParts is the queue parts and the run parts. An assembly of them reports every error of the file.
	runParts
)

// ResolveRunConfig resolves the dependency outputs the parse deferred, decodes the run parts of c with them, and
// returns the config a run reads, which can be a partial config with an error. Each call fetches the outputs again. It
// never changes c.
//
// pc gives the run's settings and sinks, and c the state of each file.
//
// Panics when c did not come from [Store.ResolveUnitConfig].
func (c *UnitConfig) ResolveRunConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
) (*TerragruntConfig, error) {
	pc = pc.forStore(v)

	return c.assemble(ctx, l, v, s, pc, keptIncludes{}, c.functions(ctx, l, v, s))
}

// assemble converts c to a config and merges in its autoinclude and included files from src, each by its merge
// strategy. It decodes the parts src decodes with the `dependency` value src gives c. It reads
// the settings and writes to the sinks of pc, reads the state of each file from the file's own config, and never
// changes c.
func (c *UnitConfig) assemble(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	src includeSource,
	fns *parseFunctions,
) (*TerragruntConfig, error) {
	pc = pc.withFile(&c.scope)
	errs := slices.Clone(c.prepErrs)

	deps, err := src.dependencies(ctx, l, v, s, pc, c)
	if err != nil {
		errs = append(errs, err)
	}

	pctx := pc.withDecodedDependencies(deps).detach()
	evalCtx := c.evalContext(fns, pctx)
	bound := c.boundFile(l, v, pc)

	parts := src.parts()
	file, diags := src.file(c, evalCtx)
	decodeErr := joinDecodeErrors(c.parseDecodeErr(parts), bound.HandleDiagnostics(diags))

	cfgFile, err := CompleteTerragruntConfigFile(
		ctx,
		l,
		v,
		pctx,
		bound,
		dependencyBlockContext(pctx, evalCtx),
		file,
		decodeErr,
	)

	err = WithNoOutputsError(v, pctx, err)
	if err != nil &&
		!slices.ContainsFunc(errs, func(seen error) bool { return errors.Is(err, seen) }) {
		errs = append(errs, err)
	}

	if cfgFile == nil {
		return nil, CouldNotResolveTerragruntConfigInFileError(c.file.ConfigPath)
	}

	cfg, err := c.convert(v, pc, pctx, cfgFile)
	if err != nil {
		errs = append(errs, err)
	}

	if parts == runParts {
		if err := ValidateDependencies(v, pctx, cfgFile.Dependencies); err != nil {
			errs = append(errs, err)
		}
	}

	if cfg != nil {
		if err := ValidateInputDefinitions(bound, cfg); err != nil {
			errs = append(errs, err)
		}

		autoMergedCfg, autoMergeErr := c.mergeAutoInclude(ctx, l, v, s, pc, src, cfg)
		if autoMergeErr != nil {
			errs = append(errs, autoMergeErr)
		}

		if autoMergeErr == nil {
			cfg = autoMergedCfg
		}
	}

	includes := pc.includes

	if includes != nil && cfg != nil {
		mergedCfg, err := c.mergeIncludes(ctx, l, v, s, pc, src, deps, cfg)
		if err != nil {
			errs = append(errs, err)

			return cfg, errors.Join(errs...)
		}

		if mergedCfg == nil {
			return cfg, errors.Join(errs...)
		}

		mergedCfg.ProcessedIncludes = includes.ByName()
		mergedCfg.Locals = cfg.Locals

		if cfg.Exclude != nil {
			mergedCfg.Exclude = cfg.Exclude
		}

		cfg = mergedCfg
	}

	if pc.include == nil && cfg != nil {
		if err := cfg.Terraform.ValidateVersion(c.file.ConfigPath); err != nil {
			errs = append(errs, err)
		}
	}

	return cfg, errors.Join(errs...)
}

// parseDecodeErr returns the error of the parse's decode that an assembly of parts reports.
func (c *UnitConfig) parseDecodeErr(parts assemblyParts) error {
	if parts == queueParts {
		return c.queueDecodeErr
	}

	return c.decodeErr
}

// convert converts cfgFile as [ConvertToTerragruntConfig] does. A config assembled for a merge gets the
// field metadata of its locals without the locals themselves, since the merged config keeps the locals of the
// including file. Locals that can fail to convert are still converted, so their error is reported.
func (c *UnitConfig) convert(
	v *venv.Venv,
	pc *ParsingContext,
	pctx *ParsingContext,
	cfgFile *TerragruntConfigFile,
) (*TerragruntConfig, error) {
	locals := pctx.Locals
	if !pc.merging || locals == nil || !ctyhelper.ConvertsToMapDirectly(*locals) {
		return ConvertToTerragruntConfig(v, pctx, c.file.ConfigPath, cfgFile)
	}

	cfg, err := ConvertToTerragruntConfig(v, pctx.WithLocals(nil), c.file.ConfigPath, cfgFile)
	if cfg == nil {
		return nil, err
	}

	metadata := map[string]any{FoundInFile: c.file.ConfigPath}

	unmarked, _ := locals.Unmark()

	for it := unmarked.ElementIterator(); it.Next(); {
		name, _ := it.Element()
		cfg.SetFieldMetadataWithType(MetadataLocals, name.AsString(), metadata)
	}

	return cfg, err
}

// boundFile returns c's file bound to a parser of pc, which logs through l and writes through v.
func (c *UnitConfig) boundFile(l log.Logger, v *venv.Venv, pc *ParsingContext) *hclparse.File {
	return c.file.Rebind(hclparse.NewParser(pc.ParserOptions(l, v)...))
}

// functions returns new functions for a parse of c's file, which use ctx, l, v, and s when called.
func (c *UnitConfig) functions(ctx context.Context, l log.Logger, v *venv.Venv, s *Store) *parseFunctions {
	return newParseFunctions(ctx, l, v, s, c.file)
}

// evalContext returns an eval context with the variables c's blocks decoded with, the `dependency` value of pctx,
// and fns, which read pctx when called. It returns nil when the eval context failed to build during the parse.
func (c *UnitConfig) evalContext(fns *parseFunctions, pctx *ParsingContext) *hcl.EvalContext {
	if c.evalVars == nil {
		return nil
	}

	vars := c.evalVars

	if pctx.DecodedDependencies != nil {
		vars = maps.Clone(vars)
		vars[MetadataDependency] = *pctx.DecodedDependencies
	}

	return fns.evalContext(pctx, vars)
}

// joinDecodeErrors returns the error [hclparse.File.HandleDiagnostics] would return for the diagnostics of the
// tagged fields and of an assembly's parts handled together. An error that holds no diagnostics stands alone.
func joinDecodeErrors(parse, deferred error) error {
	if parse == nil || deferred == nil {
		return cmp.Or(parse, deferred)
	}

	var parseDiags, deferredDiags hcl.Diagnostics

	if !errors.As(parse, &parseDiags) {
		return parse
	}

	if !errors.As(deferred, &deferredDiags) {
		return deferred
	}

	return slices.Concat(parseDiags, deferredDiags)
}

// withoutDiagnostics returns err without the diagnostics in dropped, and nil when it holds no others. An error that
// holds no diagnostics is returned unchanged.
func withoutDiagnostics(err error, dropped hcl.Diagnostics) error {
	var diags hcl.Diagnostics
	if len(dropped) == 0 || !errors.As(err, &diags) {
		return err
	}

	kept := excludeDiagnostics(diags, dropped)
	if len(kept) == 0 {
		return nil
	}

	return kept
}

// excludeDiagnostics returns the diagnostics of diags that dropped does not report, in a new slice.
func excludeDiagnostics(diags, dropped hcl.Diagnostics) hcl.Diagnostics {
	return slices.DeleteFunc(slices.Clone(diags), func(diag *hcl.Diagnostic) bool {
		return slices.ContainsFunc(dropped, func(drop *hcl.Diagnostic) bool { return sameDiagnostic(diag, drop) })
	})
}

// sameDiagnostic reports whether a and b report the same problem at the same place.
func sameDiagnostic(a, b *hcl.Diagnostic) bool {
	if a.Severity != b.Severity || a.Summary != b.Summary || a.Detail != b.Detail {
		return false
	}

	if a.Subject == nil || b.Subject == nil {
		return a.Subject == b.Subject
	}

	return *a.Subject == *b.Subject
}
