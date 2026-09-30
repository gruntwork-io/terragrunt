package config

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// includeSource is where an assembly gets the config of each file it merges.
type includeSource interface {
	// included returns the config of the file c's include block i names.
	included(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		c *UnitConfig,
		i int,
	) (*pkgconfig.TerragruntConfig, error)
	// autoIncluded returns the config of c's autoinclude at path.
	autoIncluded(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		c *UnitConfig,
		path string,
	) (*pkgconfig.TerragruntConfig, error)
}

// includeParser parses each included file with store and keeps it on the config that includes it.
type includeParser struct {
	store *hclparse.Store
}

func (p includeParser) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c *UnitConfig,
	i int,
) (*pkgconfig.TerragruntConfig, error) {
	include := c.pc.file.includes.List[i]

	included, cfg, err := c.parseIncludedConfig(ctx, l, v, p.store, &include)
	c.includedFiles[i] = newIncludedFile(included, err)

	return cfg, err
}

func (p includeParser) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	autoInclude, cfg, err := parseFile(ctx, l, v, p.store, c.pc.forAutoInclude(), path)
	c.autoInclude = newIncludedFile(autoInclude, err)

	return cfg, err
}

// keptIncludes assembles the files a parse kept. A parse keeps every file its assembly reaches, and an assembly
// from the same config reaches the same files, so a kept file is never missing.
//
// Panics when the config has no kept file for an include the assembly reaches.
type keptIncludes struct{}

func (k keptIncludes) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c *UnitConfig,
	i int,
) (*pkgconfig.TerragruntConfig, error) {
	included := c.includedFiles[i]
	if included == nil {
		panic(fmt.Sprintf("%s: no kept file for include block %d", c.file.ConfigPath, i))
	}

	if included.cfg == nil {
		return nil, included.err
	}

	cfg, err := included.cfg.assemble(ctx, l, v, k)
	if err != nil {
		include := c.pc.file.includes.List[i]

		return nil, includeParseError(err, &include, c.pc)
	}

	return cfg, nil
}

func (k keptIncludes) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	if c.autoInclude == nil {
		panic(fmt.Sprintf("%s: no kept file for autoinclude %s", c.file.ConfigPath, path))
	}

	if c.autoInclude.cfg == nil {
		return nil, c.autoInclude.err
	}

	return c.autoInclude.cfg.assemble(ctx, l, v, k)
}

// mergeIncludes merges each file c includes into cfg by its merge strategy, last include first, so later include
// blocks win.
//
// Included files parse with c's file state, so they use c's dependencies unless c's resolution returned none.
func (c *UnitConfig) mergeIncludes(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	src includeSource,
	cfg *pkgconfig.TerragruntConfig,
) (*pkgconfig.TerragruntConfig, error) {
	baseCfg := cfg

	for i, include := range slices.Backward(c.pc.file.includes.List) {
		mergeStrategy, err := include.GetMergeStrategy()
		if err != nil {
			return cfg, err
		}

		c.pc.run.FilesRead.Add(include.Path)

		includedCfg, err := src.included(ctx, l, v, c, i)
		if err != nil {
			return baseCfg, err
		}

		switch mergeStrategy {
		case pkgconfig.NoMerge:
			l.Debugf("Included config %s has strategy no merge: not merging config in.", include.Path)
		case pkgconfig.ShallowMerge:
			l.Debugf("Included config %s has strategy shallow merge: merging config in (shallow).", include.Path)

			if err := includedCfg.Merge(l, baseCfg); err != nil {
				return nil, err
			}

			baseCfg = includedCfg
		case pkgconfig.DeepMerge:
			l.Debugf("Included config %s has strategy deep merge: merging config in (deep).", include.Path)

			if err := includedCfg.DeepMerge(l, baseCfg); err != nil {
				return nil, err
			}

			baseCfg = includedCfg
		case pkgconfig.DeepMergeMapOnly:
			return nil, pkgconfig.IncludeMergeStrategyNotSupportedError(mergeStrategy)
		}
	}

	return baseCfg, nil
}

// parseIncludedConfig parses the file that include names and returns it with its config.
//
// Returns [pkgconfig.IncludedConfigMissingPathError] when include has no path, and
// [pkgconfig.IncludeConfigNotFoundError] when the file does not exist.
func (c *UnitConfig) parseIncludedConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	include *pkgconfig.IncludeConfig,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	if include.Path == "" {
		return nil, nil, pkgconfig.IncludedConfigMissingPathError(c.pc.run.TerragruntConfigPath)
	}

	hasDependency, err := pkgconfig.ConfigFileHasDependencyBlock(v.FS, include.Path)
	if err != nil {
		return nil, nil, err
	}

	includedPC := c.pc.withInclude(include)

	if hasDependency {
		includedPC = includedPC.withDiagnosticsSuppressed()
	}

	included, cfg, err := parseFile(ctx, l, v, p, includedPC, include.Path)
	if err != nil {
		return included, nil, includeParseError(err, include, c.pc)
	}

	return included, cfg, nil
}

// includeParseError reports a missing included file as [pkgconfig.IncludeConfigNotFoundError].
func includeParseError(err error, include *pkgconfig.IncludeConfig, pc *ParseContext) error {
	if _, ok := errors.AsType[pkgconfig.TerragruntConfigNotFoundError](err); ok {
		return pkgconfig.IncludeConfigNotFoundError{
			IncludePath: include.Path,
			SourcePath:  pc.run.TerragruntConfigPath,
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
	src includeSource,
	cfg *pkgconfig.TerragruntConfig,
) (*pkgconfig.TerragruntConfig, error) {
	includes := c.pc.file.includes
	if includes == nil || includes.AutoIncludePath == "" {
		return cfg, nil
	}

	autoIncludePath := includes.AutoIncludePath

	l.Debugf("Found %s, merging into unit config", autoIncludePath)

	autoIncludeCfg, err := src.autoIncluded(ctx, l, v, c, autoIncludePath)
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
