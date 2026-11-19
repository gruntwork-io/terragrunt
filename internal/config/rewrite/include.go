package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
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
	// checksDependencyDirs reports whether the assembly checks that the dependencies paths of each file are
	// directories, as [pkgconfig.ParseConfig] does.
	checksDependencyDirs() bool
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
	return c.queueFile(&c.Queue), c.queueEngineDiags
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

func (p includeResolver) checksDependencyDirs() bool {
	return false
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

// keptIncludes assembles the files a parse kept. A parse keeps the files its assembly reached before it stopped, and an
// assembly that gets further returns [UnparsedIncludeError].
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
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(ctx, l, v, pc, keptIncludes{inherited: deps})
	if err != nil {
		include := pc.file.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (k keptIncludes) checksDependencyDirs() bool {
	return true
}

func (k keptIncludes) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, pc, keptIncludes{})
}

// keptQueue assembles the files a parse kept from their queue parts alone, with no run parts and no outputs. An
// assembly that gets further than the parse did returns [UnparsedIncludeError].
type keptQueue struct{}

func (q keptQueue) file(
	c *UnitConfig,
	_ *hcl.EvalContext,
) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics) {
	return c.queueFile(&c.Queue), c.queueEngineDiags
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
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(ctx, l, v, pc, keptQueue{})
	if err != nil {
		include := pc.file.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (q keptQueue) checksDependencyDirs() bool {
	return false
}

func (q keptQueue) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, pc, keptQueue{})
}

// keptUnknownOutputs assembles the files a parse kept with `dependency` unknown, fetching no outputs. It decodes the
// run parts that a section of decodeList decodes and that read no `dependency`, and leaves out the rest. An assembly
// that gets further than the parse did returns [UnparsedIncludeError].
type keptUnknownOutputs struct {
	decodeList []pkgconfig.PartialDecodeSectionType
}

func (k keptUnknownOutputs) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*pkgconfig.TerragruntConfigFile, hcl.Diagnostics) {
	c = c.withRequestedRunParts(k.decodeList)

	run, diags := c.decodeRun(evalCtx)

	file := c.queueFile(&c.Queue)
	run.fill(c, file)

	return file, diags
}

func (k keptUnknownOutputs) dependencies(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
	_ *ParseContext,
	_ *UnitConfig,
) (*cty.Value, error) {
	return new(cty.DynamicVal), nil
}

func (k keptUnknownOutputs) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	i int,
	_ *cty.Value,
) (*pkgconfig.TerragruntConfig, error) {
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(ctx, l, v, pc, k)
	if err != nil {
		include := pc.file.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (k keptUnknownOutputs) checksDependencyDirs() bool {
	return false
}

func (k keptUnknownOutputs) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
	path string,
) (*pkgconfig.TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, pc, k)
}

// keptInclude returns the parse of the file c's include block i names.
//
// Returns the error that left the file unparsed, or [UnparsedIncludeError] when the parse stopped before reaching it.
func (c *UnitConfig) keptInclude(pc *ParseContext, i int) (*UnitConfig, error) {
	included := c.kept.included[i]
	if included == nil {
		return nil, UnparsedIncludeError{
			ConfigPath:  c.file.ConfigPath,
			IncludePath: pc.file.includes.List[i].Path,
		}
	}

	if included.cfg == nil {
		return nil, included.err
	}

	return included.cfg, nil
}

// keptAutoInclude returns the parse of c's autoinclude at path.
//
// Returns the error that left the file unparsed, or [UnparsedIncludeError] when the parse stopped before reaching it.
func (c *UnitConfig) keptAutoInclude(path string) (*UnitConfig, error) {
	autoInclude := c.kept.autoInclude
	if autoInclude == nil {
		return nil, UnparsedIncludeError{ConfigPath: c.file.ConfigPath, IncludePath: path}
	}

	if autoInclude.cfg == nil {
		return nil, autoInclude.err
	}

	return autoInclude.cfg, nil
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
	s *Store,
	include *pkgconfig.IncludeConfig,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	includedCtx, err := s.includedContext(ctx, v, pc, include)
	if err != nil {
		return nil, nil, err
	}

	included, cfg, err := s.resolveFile(ctx, l, v, includedCtx.pc, include.Path)
	if err != nil {
		return included, nil, includeParseError(err, include, pc)
	}

	return included, cfg, nil
}

// includedFileContext is how the file an include block names is parsed.
type includedFileContext struct {
	pc                 *ParseContext
	hasDependencyBlock bool
}

// includedContext returns how the file include names is parsed from pc, the context of the including file. A file
// with a dependency block parses with its diagnostics suppressed.
//
// Returns [pkgconfig.IncludedConfigMissingPathError] when include has no path.
func (s *Store) includedContext(
	ctx context.Context,
	v *venv.Venv,
	pc *ParseContext,
	include *pkgconfig.IncludeConfig,
) (includedFileContext, error) {
	if include.Path == "" {
		return includedFileContext{}, pkgconfig.IncludedConfigMissingPathError(pc.settings.TerragruntConfigPath)
	}

	hasDependencyBlock, err := s.hasDependencyBlock(ctx, v.FS, include.Path)
	if err != nil {
		return includedFileContext{}, err
	}

	includedPC := pc.forIncluded(include)

	if hasDependencyBlock {
		includedPC = includedPC.withDiagnosticsSuppressed()
	}

	return includedFileContext{pc: includedPC, hasDependencyBlock: hasDependencyBlock}, nil
}

// includedBase is the file an include block names, evaluated through its base blocks.
type includedBase struct {
	// base is the file's state through its base blocks, which may hold errors.
	base *unitBase
	// file is the file's syntax tree, bound to a parser of pc.
	file *pkghclparse.File
	// pc is the context the file was evaluated with.
	pc *ParseContext
}

// resolveIncludedBase evaluates the file include names through its base blocks, as resolving the file from pc does.
//
// Returns [pkgconfig.IncludedConfigMissingPathError] when include has no path, and
// [pkgconfig.IncludeConfigNotFoundError] when the file does not exist.
func (s *Store) resolveIncludedBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	include *pkgconfig.IncludeConfig,
) (*includedBase, error) {
	if include.Path == "" {
		return nil, pkgconfig.IncludedConfigMissingPathError(pc.settings.TerragruntConfigPath)
	}

	info, err := statConfig(v, include.Path)
	if err != nil {
		return nil, includeParseError(err, include, pc)
	}

	includedCtx, err := s.includedContext(ctx, v, pc, include)
	if err != nil {
		return nil, err
	}

	includedPC, err := includedCtx.pc.incrementDepth()
	if err != nil {
		return nil, err
	}

	parser := pkghclparse.NewParser(includedPC.parserOptions(l, v)...)

	file, err := s.ParseUnitAST(ctx, v.FS, include.Path, info, parser)
	if err != nil {
		return nil, err
	}

	base, err := s.resolveBase(ctx, l, v, includedPC, file)
	if base == nil {
		return nil, err
	}

	return &includedBase{base: base, file: file, pc: includedPC}, nil
}

// dependencyBlocks decodes the dependency blocks of the file with its base. A block decodes with `dependency`
// unknown when the parse skips resolving outputs.
//
// Returns the first error of the base when it holds one.
func (b *includedBase) dependencyBlocks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
) (pkgconfig.Dependencies, error) {
	if err := b.base.firstErr(); err != nil {
		return nil, err
	}

	pctx := b.pc.withFile(&b.base.scope).parsingContext()
	vars := b.base.evalVars

	if pctx.DecodedDependencies == nil && pctx.SkipOutputsResolution {
		vars = maps.Clone(vars)
		vars[pkgconfig.MetadataDependency] = cty.DynamicVal
	}

	evalCtx := &hcl.EvalContext{
		Functions: pkgconfig.TerragruntFunctions(ctx, l, v, pctx, b.file.ConfigPath),
		Variables: vars,
	}

	return pkgconfig.DecodeDependencyBlocks(ctx, l, v, pctx, b.file, evalCtx)
}

// includedFeatureFlags returns the feature blocks of each file a parse with pc includes, read from the base of that
// file.
func (s *Store) includedFeatureFlags(v *venv.Venv, pc *ParseContext) pkgconfig.IncludedFeatureFlags {
	return func(
		ctx context.Context,
		l log.Logger,
		_ *pkgconfig.TrackInclude,
		include *pkgconfig.IncludeConfig,
	) (pkgconfig.FeatureFlags, error) {
		included, err := s.resolveIncludedBase(ctx, l, v, pc, include)
		if err != nil {
			return nil, err
		}

		return included.base.featureBlocks, included.base.firstErr()
	}
}

// includedDependencies returns the dependency blocks of each file a parse with pc includes, decoded with the base
// of that file.
func (s *Store) includedDependencies(v *venv.Venv, pc *ParseContext) pkgconfig.IncludedDependencies {
	return func(
		ctx context.Context,
		l log.Logger,
		include *pkgconfig.IncludeConfig,
	) (pkgconfig.Dependencies, error) {
		included, err := s.resolveIncludedBase(ctx, l, v, pc, include)
		if err != nil {
			return nil, err
		}

		return included.dependencyBlocks(ctx, l, v)
	}
}

// exposedInclude returns the value of the `include` variable for each exposed include block of a parse with pc,
// converted from the config of the file the block names. Each block resolves once.
//
// The file resolves as merging it does, with its dependency outputs fetched and every run part decoded. A partial
// parse fetches no outputs of a file with a dependency or dependencies block, and decodes only the run parts of that
// file that a requested section decodes and that read no `dependency`. A partial parse of a file without such a
// block evaluates it with the IAM role options the file sets.
func (s *Store) exposedInclude(v *venv.Venv, pc *ParseContext) pkgconfig.ExposedInclude {
	type exposedValue struct {
		err   error
		value cty.Value
	}

	resolved := map[string]exposedValue{}

	return func(
		ctx context.Context,
		l log.Logger,
		_ *pkgconfig.ParsingContext,
		include *pkgconfig.IncludeConfig,
	) (cty.Value, error) {
		if kept, ok := resolved[include.Name]; ok {
			return kept.value, kept.err
		}

		value, err := s.resolveExposedInclude(ctx, l, v, pc, include)
		resolved[include.Name] = exposedValue{value: value, err: err}

		return value, err
	}
}

// resolveExposedInclude resolves the file include names from pc and converts its config for the `include` variable.
//
// Returns [pkgconfig.IncludedConfigMissingPathError] when include has no path.
func (s *Store) resolveExposedInclude(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	include *pkgconfig.IncludeConfig,
) (cty.Value, error) {
	includedCtx, err := s.includedContext(ctx, v, pc, include)
	if err != nil {
		return cty.NilVal, err
	}

	includedPC := includedCtx.pc
	partial := len(pc.settings.PartialParseDecodeList) > 0

	var (
		included *UnitConfig
		src      includeSource = keptIncludes{}
	)

	switch {
	case partial && includedCtx.hasDependencyBlock:
		included, _, err = s.resolveFile(ctx, l, v, includedPC, include.Path)
		src = keptUnknownOutputs{decodeList: pc.settings.PartialParseDecodeList}
	case partial:
		included, err = s.resolveWithFileIAMRole(ctx, l, v, includedPC, include.Path)
	default:
		included, _, err = s.resolveFile(ctx, l, v, includedPC, include.Path)
	}

	if included == nil {
		return cty.NilVal, includeParseError(err, include, pc)
	}

	cfg, err := included.assemble(ctx, l, v, includedPC, src)
	if err != nil {
		return cty.NilVal, includeParseError(err, include, pc)
	}

	return pkgconfig.TerragruntConfigAsCty(cfg)
}

// resolveWithFileIAMRole resolves the file at cfgPath with pc, evaluating it with the IAM role options its config
// sets, as [pkgconfig.ParseConfig] resolves them. The resolution with the role options of pc serves when they match.
//
// Returns no parse with the error when the resolution with the role options of pc produced no config.
func (s *Store) resolveWithFileIAMRole(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, error) {
	c, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)
	if cfg == nil {
		return nil, err
	}

	pctx := pc.parsingContext()

	iamRole := pkgconfig.FileIAMRoleOptions(pctx, cfg.GetIAMRoleOptions())
	if iamRole == pctx.IAMRoleOptions {
		return c, nil
	}

	c, _, err = s.resolveFile(ctx, l, v, pc.forIAMRole(iamRole), cfgPath)

	return c, err
}

// includeParseError reports a missing included file as [pkgconfig.IncludeConfigNotFoundError].
func includeParseError(err error, include *pkgconfig.IncludeConfig, pc *ParseContext) error {
	if errors.As(err, new(pkgconfig.TerragruntConfigNotFoundError)) {
		return pkgconfig.IncludeConfigNotFoundError{
			IncludePath: include.Path,
			SourcePath:  pc.settings.TerragruntConfigPath,
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
