package config

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// includeSource is where an assembly gets the config of each file it merges, and whether it decodes the run parts of
// each one: the parse presents the queue parts it decoded, and ToV1 adds the run parts.
type includeSource interface {
	// file builds the v1 file of c from its queue parts, decoding the run parts with evalCtx when the assembly
	// reaches them, and returns the diagnostics of that decode.
	file(c *UnitConfig, evalCtx *hcl.EvalContext) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics)
	// dependencies returns the `dependency` value c decodes with.
	dependencies(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		pc *ParseContext,
		c *UnitConfig,
	) (*cty.Value, error)
	// included returns the config of the file c's include block i names, which decodes with deps, the `dependency`
	// value of c, and resolves its own dependencies when deps is nil.
	included(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		pc *ParseContext,
		c *UnitConfig,
		i int,
		deps *cty.Value,
	) (*pkgconfig.TerragruntConfig, error)
	// autoIncluded returns the config of c's autoinclude at path.
	autoIncluded(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		pc *ParseContext,
		c *UnitConfig,
		path string,
	) (*pkgconfig.TerragruntConfig, error)
}

// includeResolver resolves each included file with store and records it in kept.
type includeResolver struct {
	store *Store
	kept  *keptFiles
}

func (p includeResolver) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics) {
	return c.queueFile(&c.Queue), nil
}

func (p includeResolver) dependencies(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
	pc *ParseContext,
	_ *UnitConfig,
) (*cty.Value, error) {
	return pc.file.decodedDeps, nil
}

func (p includeResolver) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	i int,
	_ *cty.Value,
) (*pkgconfig.TerragruntConfig, error) {
	include := pc.file.includes.List[i]

	included, cfg, err := c.resolveIncludedConfig(ctx, l, v, pc, p.store, &include)
	p.kept.included[i] = newIncludedFile(included, err)

	return cfg, err
}

func (p includeResolver) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	autoInclude, cfg, err := p.store.resolveFile(ctx, l, v, pc.forAutoInclude(), path)
	p.kept.autoInclude = newIncludedFile(autoInclude, err)

	return cfg, err
}

// keptIncludes assembles the files a parse kept. A parse keeps every file its assembly reaches, and an assembly
// from the same config reaches the same files, so a kept file is never missing.
//
// Panics when the config has no kept file for an include the assembly reaches.
type keptIncludes struct {
	// inherited is the `dependency` value of the including file, nil when the assembled file resolves its own.
	inherited *cty.Value
}

func (k keptIncludes) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics) {
	run, diags := c.decodeRun(evalCtx)

	file := c.queueFile(&c.Queue)
	run.fill(c, file)

	return file, diags
}

func (k keptIncludes) dependencies(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
) (*cty.Value, error) {
	if k.inherited != nil {
		return k.inherited, nil
	}

	return c.dependencyValue(ctx, l, v, pc)
}

func (k keptIncludes) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	i int,
	deps *cty.Value,
) (*pkgconfig.TerragruntConfig, error) {
	included := c.kept.included[i]
	if included == nil {
		panic(fmt.Sprintf("%s: no kept file for include block %d", c.file.ConfigPath, i))
	}

	if included.cfg == nil {
		return nil, included.err
	}

	cfg, err := included.cfg.assemble(ctx, l, v, pc, keptIncludes{inherited: deps})
	if err != nil {
		include := pc.file.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (k keptIncludes) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	if c.kept.autoInclude == nil {
		panic(fmt.Sprintf("%s: no kept file for autoinclude %s", c.file.ConfigPath, path))
	}

	if c.kept.autoInclude.cfg == nil {
		return nil, c.kept.autoInclude.err
	}

	return c.kept.autoInclude.cfg.assemble(ctx, l, v, pc, keptIncludes{})
}

// keptQueue assembles the files a parse kept from their queue parts alone, with no run parts and no outputs.
//
// Panics when the config has no kept file for an include the assembly reaches.
type keptQueue struct{}

func (q keptQueue) file(
	c *UnitConfig,
	_ *hcl.EvalContext,
) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics) {
	return c.queueFile(&c.Queue), nil
}

func (q keptQueue) dependencies(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
	pc *ParseContext,
	_ *UnitConfig,
) (*cty.Value, error) {
	return pc.file.decodedDeps, nil
}

func (q keptQueue) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	i int,
	_ *cty.Value,
) (*pkgconfig.TerragruntConfig, error) {
	included := c.kept.included[i]
	if included == nil {
		panic(fmt.Sprintf("%s: no kept file for include block %d", c.file.ConfigPath, i))
	}

	if included.cfg == nil {
		return nil, included.err
	}

	cfg, err := included.cfg.assemble(ctx, l, v, pc, keptQueue{})
	if err != nil {
		include := pc.file.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (q keptQueue) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	if c.kept.autoInclude == nil {
		panic(fmt.Sprintf("%s: no kept file for autoinclude %s", c.file.ConfigPath, path))
	}

	if c.kept.autoInclude.cfg == nil {
		return nil, c.kept.autoInclude.err
	}

	return c.kept.autoInclude.cfg.assemble(ctx, l, v, pc, keptQueue{})
}

// mergeIncludes merges each file c includes into cfg by its merge strategy, last include first, so later include
// blocks win. Each included file decodes with deps, the `dependency` value of c.
func (c *UnitConfig) mergeIncludes(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	src includeSource,
	deps *cty.Value,
	cfg *pkgconfig.TerragruntConfig,
) (*pkgconfig.TerragruntConfig, error) {
	baseCfg := cfg

	for i, include := range slices.Backward(pc.file.includes.List) {
		mergeStrategy, err := include.GetMergeStrategy()
		if err != nil {
			return cfg, err
		}

		pc.sinks.filesRead.Add(include.Path)

		switch mergeStrategy {
		case pkgconfig.NoMerge:
			l.Debugf(
				"Included config %s has strategy no merge: not merging config in.",
				include.Path,
			)

			continue
		case pkgconfig.ShallowMerge:
			l.Debugf(
				"Included config %s has strategy shallow merge: merging config in (shallow).",
				include.Path,
			)
		case pkgconfig.DeepMerge:
			l.Debugf(
				"Included config %s has strategy deep merge: merging config in (deep).",
				include.Path,
			)
		case pkgconfig.DeepMergeMapOnly:
			return nil, pkgconfig.IncludeMergeStrategyNotSupportedError(mergeStrategy)
		}

		includedCfg, err := src.included(ctx, l, v, pc, c, i, deps)
		if err != nil {
			return baseCfg, err
		}

		if err := mergeIncludedConfig(l, mergeStrategy, includedCfg, baseCfg); err != nil {
			return nil, err
		}

		baseCfg = includedCfg
	}

	return baseCfg, nil
}

// mergeIncludedConfig merges base into included with mergeStrategy, which is ShallowMerge or DeepMerge.
func mergeIncludedConfig(
	l log.Logger,
	mergeStrategy pkgconfig.MergeStrategyType,
	included, base *pkgconfig.TerragruntConfig,
) error {
	if mergeStrategy == pkgconfig.ShallowMerge {
		return included.Merge(l, base)
	}

	return included.DeepMerge(l, base)
}

// resolveIncludedConfig resolves the file that include names and returns it with its config.
//
// Returns [pkgconfig.IncludedConfigMissingPathError] when include has no path, and
// [pkgconfig.IncludeConfigNotFoundError] when the file does not exist.
func (c *UnitConfig) resolveIncludedConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	p *Store,
	include *pkgconfig.IncludeConfig,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	if include.Path == "" {
		return nil, nil, pkgconfig.IncludedConfigMissingPathError(pc.inputs.TerragruntConfigPath)
	}

	hasDependency, err := pkgconfig.ConfigFileHasDependencyBlock(v.FS, include.Path)
	if err != nil {
		return nil, nil, err
	}

	includedPC := pc.forIncluded(include)

	if hasDependency {
		includedPC = includedPC.withDiagnosticsSuppressed()
	}

	included, cfg, err := p.resolveFile(ctx, l, v, includedPC, include.Path)
	if err != nil {
		return included, nil, includeParseError(err, include, pc)
	}

	return included, cfg, nil
}

// includeParseError reports a missing included file as [pkgconfig.IncludeConfigNotFoundError].
func includeParseError(err error, include *pkgconfig.IncludeConfig, pc *ParseContext) error {
	if errors.As(err, new(pkgconfig.TerragruntConfigNotFoundError)) {
		return pkgconfig.IncludeConfigNotFoundError{
			IncludePath: include.Path,
			SourcePath:  pc.inputs.TerragruntConfigPath,
		}
	}

	return err
}

// mergeAutoInclude shallow merges c's autoinclude into cfg, the autoinclude winning, and returns cfg unchanged
// when there is none. The autoinclude resolves its own dependencies and merges no autoinclude of its own.
func (c *UnitConfig) mergeAutoInclude(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	src includeSource,
	cfg *pkgconfig.TerragruntConfig,
) (*pkgconfig.TerragruntConfig, error) {
	includes := pc.file.includes
	if includes == nil || includes.AutoIncludePath == "" {
		return cfg, nil
	}

	autoIncludePath := includes.AutoIncludePath

	l.Debugf("Found %s, merging into unit config", autoIncludePath)

	autoIncludeCfg, err := src.autoIncluded(ctx, l, v, pc, c, autoIncludePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", autoIncludePath, err)
	}

	if err := cfg.Merge(l, autoIncludeCfg); err != nil {
		return nil, fmt.Errorf("failed to merge %s: %w", autoIncludePath, err)
	}

	return cfg, nil
}

// newIncludedFile keeps cfg, or err when the parse failed before decoding the file.
func newIncludedFile(cfg *UnitConfig, err error) *includedFile {
	if cfg == nil {
		return &includedFile{err: err}
	}

	return &includedFile{cfg: cfg}
}
