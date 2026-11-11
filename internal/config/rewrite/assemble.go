package config

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ToV1 resolves the dependency outputs the parse deferred, decodes the run parts of c with them, and returns the
// config and errors that [pkgconfig.ParseConfigFile] returns for the same file, which can be a partial config with
// an error. Each call fetches the outputs again. It never changes c.
//
// pc gives the run's settings and sinks, and c the state of each file.
//
// Panics when c did not come from [Store.ResolveUnitConfig].
func (c *UnitConfig) ToV1(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
) (*pkgconfig.TerragruntConfig, error) {
	return c.assemble(ctx, l, v, pc, keptIncludes{})
}

// assemble converts c to a config and merges in its autoinclude and included files from src, as
// [pkgconfig.ParseConfig] does. It decodes the parts src decodes with the `dependency` value src gives c. It reads
// the settings and writes to the sinks of pc, reads the state of each file from the file's own config, and never
// changes c.
func (c *UnitConfig) assemble(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	src includeSource,
) (*pkgconfig.TerragruntConfig, error) {
	pc = pc.withFile(&c.scope)
	errs := slices.Clone(c.prepErrs)

	deps, err := src.dependencies(ctx, l, v, pc, c)
	if err != nil {
		errs = append(errs, err)
	}

	pctx := pc.withDecodedDependencies(deps).parsingContext()
	evalCtx := c.evalContext(ctx, l, v, pctx)
	bound := c.boundFile(l, v, pc)

	file, diags := src.file(c, evalCtx)
	decodeErr := joinDecodeErrors(c.decodeErr, bound.HandleDiagnostics(diags))

	cfgFile, err := pkgconfig.CompleteTerragruntConfigFile(
		ctx,
		l,
		v,
		pctx,
		bound,
		evalCtx,
		file,
		decodeErr,
	)
	if err != nil &&
		!slices.ContainsFunc(errs, func(seen error) bool { return errors.Is(err, seen) }) {
		errs = append(errs, err)
	}

	if cfgFile == nil {
		return nil, pkgconfig.CouldNotResolveTerragruntConfigInFileError(c.file.ConfigPath)
	}

	cfg, err := pkgconfig.ConvertToTerragruntConfig(v, pctx, c.file.ConfigPath, cfgFile)
	if err != nil {
		errs = append(errs, err)
	}

	if cfg != nil {
		if err := pkgconfig.ValidateInputDefinitions(bound, cfg); err != nil {
			errs = append(errs, err)
		}

		autoMergedCfg, autoMergeErr := c.mergeAutoInclude(ctx, l, v, pc, src, cfg)
		if autoMergeErr != nil {
			errs = append(errs, autoMergeErr)
		}

		if autoMergeErr == nil {
			cfg = autoMergedCfg
		}
	}

	includes := pc.file.includes

	if includes != nil && cfg != nil {
		mergedCfg, err := c.mergeIncludes(ctx, l, v, pc, src, deps, cfg)
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

	if pc.file.include == nil && cfg != nil {
		if err := cfg.Terraform.ValidateVersion(c.file.ConfigPath); err != nil {
			errs = append(errs, err)
		}
	}

	return cfg, errors.Join(errs...)
}

// boundFile returns c's file bound to a parser of pc, which logs through l and writes through v.
func (c *UnitConfig) boundFile(l log.Logger, v *venv.Venv, pc *ParseContext) *pkghclparse.File {
	return c.file.Rebind(pkghclparse.NewParser(pc.parserOptions(l, v)...))
}

// evalContext returns an eval context with the variables c's blocks decoded with, the `dependency` value of pctx,
// and functions built for ctx, l, v, and pctx. It returns nil when the eval context failed to build during the
// parse.
func (c *UnitConfig) evalContext(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
) *hcl.EvalContext {
	if c.evalVars == nil {
		return nil
	}

	vars := c.evalVars

	if pctx.DecodedDependencies != nil {
		vars = maps.Clone(vars)
		vars[pkgconfig.MetadataDependency] = *pctx.DecodedDependencies
	}

	return &hcl.EvalContext{
		Functions: pkgconfig.TerragruntFunctions(ctx, l, v, pctx, c.file.ConfigPath),
		Variables: vars,
	}
}

// joinDecodeErrors returns the error [pkghclparse.File.HandleDiagnostics] would return for the diagnostics of the
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
