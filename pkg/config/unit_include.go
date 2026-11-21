package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// includeSource is where an assembly gets the config of each file it merges, and whether it decodes the run parts of
// each one: the parse presents the queue parts it decoded, and ToV1 adds the run parts.
type includeSource interface {
	// parts returns the parts of each file the assembly decodes.
	parts() assemblyParts
	// file builds the v1 file of c from its queue parts, decoding the run parts with evalCtx when the assembly
	// reaches them, and returns the diagnostics of that decode.
	file(c *UnitConfig, evalCtx *hcl.EvalContext) (*TerragruntConfigFile, hcl.Diagnostics)
	// dependencies returns the `dependency` value c decodes with and the blocks it was resolved from.
	dependencies(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		s *Store,
		pc *ParsingContext,
		c *UnitConfig,
	) (*dependencyResolution, error)
	// included returns the config of the file c's include block i names, which decodes with deps, c's `dependency`
	// value and the blocks it was resolved from, and resolves its own dependencies when deps is nil.
	included(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		s *Store,
		pc *ParsingContext,
		c *UnitConfig,
		i int,
		deps *dependencyResolution,
	) (*TerragruntConfig, error)
	// autoIncluded returns the config of c's autoinclude at path.
	autoIncluded(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		s *Store,
		pc *ParsingContext,
		c *UnitConfig,
		path string,
	) (*TerragruntConfig, error)
}

// includeResolver resolves each included file and records it in kept.
type includeResolver struct {
	kept *keptFiles
}

func (p includeResolver) parts() assemblyParts {
	return queueParts
}

func (p includeResolver) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*TerragruntConfigFile, hcl.Diagnostics) {
	return c.queueFile(&c.Queue), c.queueEngineDiags
}

func (p includeResolver) dependencies(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
	s *Store,
	pc *ParsingContext,
	_ *UnitConfig,
) (*dependencyResolution, error) {
	return pc.decodedDependencies(), nil
}

func (p includeResolver) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	i int,
	_ *dependencyResolution,
) (*TerragruntConfig, error) {
	include := pc.includes.List[i]

	included, cfg, err := c.resolveIncludedConfig(ctx, l, v, pc, s, &include)
	p.kept.included[i] = newIncludedFile(included, err)

	return cfg, err
}

func (p includeResolver) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	path string,
) (*TerragruntConfig, error) {
	autoInclude, cfg, err := s.resolveFile(ctx, l, v, pc.forAutoInclude(), path)
	p.kept.autoInclude = newIncludedFile(autoInclude, err)

	return cfg, err
}

// keptIncludes assembles the files a parse kept. A parse keeps the files its assembly reached before it stopped, and an
// assembly that gets further returns [UnparsedIncludeError].
type keptIncludes struct {
	// inherited is the `dependency` value of the including file and the blocks it was resolved from, nil when the
	// assembled file resolves its own.
	inherited *dependencyResolution
}

func (k keptIncludes) parts() assemblyParts {
	return runParts
}

func (k keptIncludes) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*TerragruntConfigFile, hcl.Diagnostics) {
	run, diags := c.decodeRun(evalCtx)

	file := c.queueFile(&c.Queue)
	run.fill(c, file)

	return file, diags
}

func (k keptIncludes) dependencies(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
) (*dependencyResolution, error) {
	if k.inherited != nil {
		return k.inherited, nil
	}

	return c.dependencyValue(ctx, l, v, s, pc)
}

func (k keptIncludes) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	i int,
	deps *dependencyResolution,
) (*TerragruntConfig, error) {
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(
		ctx,
		l,
		v,
		s,
		pc.forMerge(),
		keptIncludes{inherited: deps},
		included.functions(ctx, l, v, s),
	)
	if err != nil {
		include := pc.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (k keptIncludes) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	path string,
) (*TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, s, pc, keptIncludes{}, autoInclude.functions(ctx, l, v, s))
}

// keptQueue assembles the files a parse kept from their queue parts alone, with no run parts and no outputs. An
// assembly that gets further than the parse did returns [UnparsedIncludeError].
type keptQueue struct{}

func (q keptQueue) parts() assemblyParts {
	return queueParts
}

func (q keptQueue) file(
	c *UnitConfig,
	_ *hcl.EvalContext,
) (*TerragruntConfigFile, hcl.Diagnostics) {
	return c.queueFile(&c.Queue), c.queueEngineDiags
}

func (q keptQueue) dependencies(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
	s *Store,
	pc *ParsingContext,
	_ *UnitConfig,
) (*dependencyResolution, error) {
	return pc.decodedDependencies(), nil
}

func (q keptQueue) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	i int,
	_ *dependencyResolution,
) (*TerragruntConfig, error) {
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(ctx, l, v, s, pc.forMerge(), keptQueue{}, included.functions(ctx, l, v, s))
	if err != nil {
		include := pc.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (q keptQueue) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	path string,
) (*TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, s, pc, keptQueue{}, autoInclude.functions(ctx, l, v, s))
}

// keptUnknownOutputs assembles the files a parse kept with `dependency` unknown, fetching no outputs. It decodes the
// run parts that a section of decodeList decodes and that read no `dependency`, and leaves out the rest. An assembly
// that gets further than the parse did returns [UnparsedIncludeError].
type keptUnknownOutputs struct {
	// readers collects a [PartialParseReadsDependencyError] for each file whose parts it leaves out for reading
	// `dependency`, and is nil to collect none.
	readers    *[]error
	decodeList []PartialDecodeSectionType
}

func (k keptUnknownOutputs) parts() assemblyParts {
	return queueParts
}

func (k keptUnknownOutputs) file(
	c *UnitConfig,
	evalCtx *hcl.EvalContext,
) (*TerragruntConfigFile, hcl.Diagnostics) {
	if parts := c.requestedDependencyReaders(k.decodeList); k.readers != nil && len(parts) > 0 {
		*k.readers = append(*k.readers, PartialParseReadsDependencyError{ConfigPath: c.file.ConfigPath, Parts: parts})
	}

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
	s *Store,
	_ *ParsingContext,
	_ *UnitConfig,
) (*dependencyResolution, error) {
	return &dependencyResolution{value: new(cty.DynamicVal)}, nil
}

func (k keptUnknownOutputs) included(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	i int,
	_ *dependencyResolution,
) (*TerragruntConfig, error) {
	included, err := c.keptInclude(pc, i)
	if err != nil {
		return nil, err
	}

	cfg, err := included.assemble(ctx, l, v, s, pc, k, included.functions(ctx, l, v, s))
	if err != nil {
		include := pc.includes.List[i]

		return nil, includeParseError(err, &include, pc)
	}

	return cfg, nil
}

func (k keptUnknownOutputs) autoIncluded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	c *UnitConfig,
	path string,
) (*TerragruntConfig, error) {
	autoInclude, err := c.keptAutoInclude(path)
	if err != nil {
		return nil, err
	}

	return autoInclude.assemble(ctx, l, v, s, pc, k, autoInclude.functions(ctx, l, v, s))
}

// keptInclude returns the parse of the file c's include block i names.
//
// Returns the error that left the file unparsed, or [UnparsedIncludeError] when the parse stopped before reaching it.
func (c *UnitConfig) keptInclude(pc *ParsingContext, i int) (*UnitConfig, error) {
	included := c.kept.included[i]
	if included == nil {
		return nil, UnparsedIncludeError{
			ConfigPath:  c.file.ConfigPath,
			IncludePath: pc.includes.List[i].Path,
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
	s *Store,
	pc *ParsingContext,
	src includeSource,
	deps *dependencyResolution,
	cfg *TerragruntConfig,
) (*TerragruntConfig, error) {
	baseCfg := cfg

	for i, include := range slices.Backward(pc.includes.List) {
		mergeStrategy, err := include.GetMergeStrategy()
		if err != nil {
			return cfg, err
		}

		pc.FilesRead.Add(include.Path)

		switch mergeStrategy {
		case NoMerge:
			l.Debugf(
				"Included config %s has strategy no merge: not merging config in.",
				include.Path,
			)

			continue
		case ShallowMerge:
			l.Debugf(
				"Included config %s has strategy shallow merge: merging config in (shallow).",
				include.Path,
			)
		case DeepMerge:
			l.Debugf(
				"Included config %s has strategy deep merge: merging config in (deep).",
				include.Path,
			)
		case DeepMergeMapOnly:
			return nil, IncludeMergeStrategyNotSupportedError(mergeStrategy)
		}

		includedCfg, err := src.included(ctx, l, v, s, pc, c, i, deps)
		if err != nil {
			return baseCfg, err
		}

		if err := mergeIncludedUnitConfig(l, mergeStrategy, includedCfg, baseCfg); err != nil {
			return nil, err
		}

		baseCfg = includedCfg
	}

	return baseCfg, nil
}

// mergeIncludedUnitConfig merges base into included with mergeStrategy, which is ShallowMerge or DeepMerge.
func mergeIncludedUnitConfig(
	l log.Logger,
	mergeStrategy MergeStrategyType,
	included, base *TerragruntConfig,
) error {
	if mergeStrategy == ShallowMerge {
		return included.Merge(l, base)
	}

	return included.DeepMerge(l, base)
}

// resolveIncludedConfig resolves the file that include names and returns it with its config, for merging into the
// config of c.
//
// Returns [IncludedConfigMissingPathError] when include has no path, and
// [IncludeConfigNotFoundError] when the file does not exist.
func (c *UnitConfig) resolveIncludedConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	s *Store,
	include *IncludeConfig,
) (*UnitConfig, *TerragruntConfig, error) {
	includedCtx, err := s.includedContext(ctx, v, pc, include)
	if err != nil {
		return nil, nil, err
	}

	included, cfg, err := s.resolveFile(ctx, l, v, includedCtx.pc.forMerge(), include.Path)
	if err != nil {
		return included, nil, includeParseError(err, include, pc)
	}

	return included, cfg, nil
}

// includedFileContext is how the file an include block names is parsed.
type includedFileContext struct {
	pc                 *ParsingContext
	hasDependencyBlock bool
}

// includedContext returns how the file include names is parsed from pc, the context of the including file. A file
// with a dependency block parses with its diagnostics suppressed.
//
// Returns [IncludedConfigMissingPathError] when include has no path.
func (s *Store) includedContext(
	ctx context.Context,
	v *venv.Venv,
	pc *ParsingContext,
	include *IncludeConfig,
) (includedFileContext, error) {
	if include.Path == "" {
		return includedFileContext{}, IncludedConfigMissingPathError(pc.TerragruntConfigPath)
	}

	hasDependencyBlock, err := s.hasDependencyBlock(ctx, v.FS, include.Path)
	if err != nil {
		return includedFileContext{}, err
	}

	includedPC := pc.forIncluded(include)

	if hasDependencyBlock {
		includedPC = includedPC.forDiagnosticsSuppressed()
	}

	return includedFileContext{pc: includedPC, hasDependencyBlock: hasDependencyBlock}, nil
}

// includedBase is the file an include block names, evaluated through its base blocks.
type includedBase struct {
	// base is the file's state through its base blocks, which may hold errors.
	base *unitBase
	// file is the file's syntax tree, bound to a parser of pc.
	file *hclparse.File
	// pc is the context the file was evaluated with.
	pc *ParsingContext
}

// resolveIncludedBase evaluates the file include names through its base blocks, as resolving the file from pc does.
//
// Returns [IncludedConfigMissingPathError] when include has no path, and
// [IncludeConfigNotFoundError] when the file does not exist.
func (s *Store) resolveIncludedBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	include *IncludeConfig,
) (*includedBase, error) {
	if include.Path == "" {
		return nil, IncludedConfigMissingPathError(pc.TerragruntConfigPath)
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

	parser := hclparse.NewParser(includedPC.ParserOptions(l, v)...)

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
	s *Store,
) (Dependencies, error) {
	if err := b.base.firstErr(); err != nil {
		return nil, err
	}

	pctx := b.pc.withFile(&b.base.scope).detach()
	vars := b.base.evalVars

	if pctx.DecodedDependencies == nil && pctx.SkipOutputsResolution {
		vars = maps.Clone(vars)
		vars[MetadataDependency] = cty.DynamicVal
	}

	evalCtx := &hcl.EvalContext{Variables: vars}
	if declaresDependencies(b.file.Body) {
		evalCtx = TerragruntEvalContext(ctx, l, v, s, pctx, b.file.ConfigPath, b.file.FunctionCalls(), vars)
	}

	return DecodeDependencyBlocks(ctx, l, v, pctx, b.file, evalCtx)
}

// includedFeatureFlags returns the feature blocks of each file a parse with pc includes, read from the base of that
// file.
func (s *Store) includedFeatureFlags(v *venv.Venv, pc *ParsingContext) IncludedFeatureFlags {
	return func(
		ctx context.Context,
		l log.Logger,
		_ *TrackInclude,
		include *IncludeConfig,
	) (FeatureFlags, error) {
		included, err := s.resolveIncludedBase(ctx, l, v, pc, include)
		if err != nil {
			return nil, err
		}

		return included.base.featureBlocks, included.base.firstErr()
	}
}

// includedDependencies returns the dependency blocks of each file a parse with pc includes, decoded with the base
// of that file.
func (s *Store) includedDependencies(v *venv.Venv, pc *ParsingContext) IncludedDependencies {
	return func(
		ctx context.Context,
		l log.Logger,
		include *IncludeConfig,
	) (Dependencies, error) {
		included, err := s.resolveIncludedBase(ctx, l, v, pc, include)
		if err != nil {
			return nil, err
		}

		return included.dependencyBlocks(ctx, l, v, s)
	}
}

// exposedInclude returns the value of the `include` variable for each exposed include block of a parse with pc,
// converted from the config of the file the block names. Each block resolves once.
//
// The file resolves as merging it does, with its dependency outputs fetched and every run part decoded. A partial
// parse fetches no outputs of a file with a dependency or dependencies block, and decodes only the run parts of that
// file that read no `dependency`, whatever the decode list, so every partial parse shares one resolution. A partial
// parse of a file without such a block evaluates it with the IAM role options the file sets.
//
// The second function returns the errors of the blocks that failed to resolve, in name order, each annotated by
// [ExposedIncludeError].
func (s *Store) exposedInclude(v *venv.Venv, pc *ParsingContext) (ExposedInclude, func() error) {
	type exposedValue struct {
		err     error
		include *IncludeConfig
		value   cty.Value
	}

	resolved := map[string]exposedValue{}

	exposed := func(
		ctx context.Context,
		l log.Logger,
		_ *ParsingContext,
		include *IncludeConfig,
	) (cty.Value, error) {
		if kept, ok := resolved[include.Name]; ok {
			return kept.value, kept.err
		}

		value, err := s.resolveExposedInclude(ctx, l, v, pc, include)
		resolved[include.Name] = exposedValue{err: err, include: include, value: value}

		return value, err
	}

	failed := func() error {
		var errs []error

		for _, name := range slices.Sorted(maps.Keys(resolved)) {
			if kept := resolved[name]; kept.err != nil {
				errs = append(errs, ExposedIncludeError(kept.include, kept.err))
			}
		}

		return errors.Join(errs...)
	}

	return exposed, failed
}

// resolveExposedInclude resolves the file include names from pc and converts its config for the `include` variable.
//
// Returns [IncludedConfigMissingPathError] when include has no path.
func (s *Store) resolveExposedInclude(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	include *IncludeConfig,
) (cty.Value, error) {
	includedCtx, err := s.includedContext(ctx, v, pc, include)
	if err != nil {
		return cty.NilVal, err
	}

	includedPC := includedCtx.pc
	partial := len(pc.PartialParseDecodeList) > 0

	var (
		included *UnitConfig
		src      includeSource = keptIncludes{}
	)

	switch {
	case partial && includedCtx.hasDependencyBlock:
		included, _, err = s.resolveFile(ctx, l, v, includedPC, include.Path)
		src = keptUnknownOutputs{decodeList: runSections}
	case partial:
		included, err = s.resolveWithFileIAMRole(ctx, l, v, includedPC, include.Path)
	default:
		included, _, err = s.resolveFile(ctx, l, v, includedPC, include.Path)
	}

	if included == nil {
		return cty.NilVal, includeParseError(err, include, pc)
	}

	cfg, err := included.assemble(ctx, l, v, s, includedPC, src, included.functions(ctx, l, v, s))
	if err != nil {
		return cty.NilVal, includeParseError(err, include, pc)
	}

	return TerragruntConfigAsCty(cfg)
}

// resolveWithFileIAMRole resolves the file at cfgPath with pc, evaluating it with the IAM role options its config
// sets, as a full parse does: from its iam_role and iam_web_identity_token, under the options passed on the CLI. The
// resolution with the role options of pc serves when they match.
//
// Returns no parse with the error when the resolution with the role options of pc produced no config.
func (s *Store) resolveWithFileIAMRole(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*UnitConfig, error) {
	c, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)
	if cfg == nil {
		return nil, err
	}

	pctx := pc.detach()

	fileOptions := iam.RoleOptions{RoleARN: cfg.IamRole, WebIdentityToken: cfg.IamWebIdentityToken}

	iamRole := FileIAMRoleOptions(pctx, fileOptions)
	if iamRole == pctx.IAMRoleOptions {
		return c, nil
	}

	c, _, err = s.resolveFile(ctx, l, v, pc.forIAMRole(iamRole), cfgPath)

	return c, err
}

// includeParseError reports a missing included file as [IncludeConfigNotFoundError].
func includeParseError(err error, include *IncludeConfig, pc *ParsingContext) error {
	if errors.As(err, new(TerragruntConfigNotFoundError)) {
		return IncludeConfigNotFoundError{
			IncludePath: include.Path,
			SourcePath:  pc.TerragruntConfigPath,
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
	s *Store,
	pc *ParsingContext,
	src includeSource,
	cfg *TerragruntConfig,
) (*TerragruntConfig, error) {
	includes := pc.includes
	if includes == nil || includes.AutoIncludePath == "" {
		return cfg, nil
	}

	autoIncludePath := includes.AutoIncludePath

	l.Debugf("Found %s, merging into unit config", autoIncludePath)

	autoIncludeCfg, err := src.autoIncluded(ctx, l, v, s, pc, c, autoIncludePath)
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
