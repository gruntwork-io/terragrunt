package config

import (
	"context"
	"errors"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ToV1 returns the config and errors that [pkgconfig.ParseConfigFile] returns for the same file, which can be a
// partial config with an error. It never changes c.
//
// Panics when c did not come from [ParseConfig] or [ParseConfigFile].
func (c *UnitConfig) ToV1(ctx context.Context, l log.Logger, v *venv.Venv) (*pkgconfig.TerragruntConfig, error) {
	return c.assemble(ctx, l, v, keptIncludes{})
}

// assemble converts c to a config and merges in its autoinclude and included files from src, as
// [pkgconfig.ParseConfig] does.
func (c *UnitConfig) assemble(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	src includeSource,
) (*pkgconfig.TerragruntConfig, error) {
	errs := slices.Clone(c.prepErrs)
	pctx := c.pc.parsingContext()

	evalCtx := c.evalContext(ctx, l, v, pctx)

	cfgFile, err := pkgconfig.CompleteTerragruntConfigFile(ctx, l, v, pctx, c.file, evalCtx, c.v1File(), c.decodeErr)
	if err != nil {
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
		autoMergedCfg, autoMergeErr := c.mergeAutoInclude(ctx, l, v, src, cfg)
		if autoMergeErr != nil {
			errs = append(errs, autoMergeErr)
		}

		if autoMergeErr == nil {
			cfg = autoMergedCfg
		}
	}

	includes := c.pc.file.includes

	if includes != nil && cfg != nil {
		mergedCfg, err := c.mergeIncludes(ctx, l, v, src, cfg)
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

	if c.pc.file.include == nil && cfg != nil {
		if err := cfg.Terraform.ValidateVersion(c.file.ConfigPath); err != nil {
			errs = append(errs, err)
		}
	}

	return cfg, errors.Join(errs...)
}

// evalContext returns an eval context with the variables c's blocks decoded with and functions built for ctx, l, v,
// and pctx. It returns nil when the eval context failed to build during the parse.
func (c *UnitConfig) evalContext(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
) *hcl.EvalContext {
	if c.evalVars == nil {
		return nil
	}

	return &hcl.EvalContext{
		Functions: pkgconfig.TerragruntFunctions(ctx, l, v, pctx, c.file.ConfigPath),
		Variables: c.evalVars,
	}
}
