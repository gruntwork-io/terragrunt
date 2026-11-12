package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResolveUnitConfig resolves the config file at cfgPath and the files it merges in, fetching no dependency outputs.
//
// The result is memoized in s by file and context: a second call for the same file and context shares the first's
// evaluation. [Store.Invalidate] finds a memoized result by the config files it parsed, and by a file it read through
// a function such as file only when pc's run records its reads.
//
// The [UnitConfig] is shared with every caller of the same file and context, and must not be changed.
//
// Returns the errors of the parse, which [UnitConfig.ToV1] returns again with those of resolving the outputs. Returns
// nil when the parse produced no config.
func (s *Store) ResolveUnitConfig(
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

	return c, err
}

// ResolveUnitBaseVariables returns the variables the file at cfgPath evaluates its blocks with: its unit values and
// the locals, feature, and include blocks [pkgconfig.DecodeBaseBlocks] decodes. It stops before the dependency blocks
// that order the queue.
//
// The variables are memoized in s by file and context, and [Store.ResolveUnitQueueConfig] builds on them. Each call
// gets a map of its own.
//
// Returns the variables with an error when an earlier step fails but the eval context still builds.
func (s *Store) ResolveUnitBaseVariables(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (map[string]cty.Value, error) {
	pc, err := pc.incrementDepth()
	if err != nil {
		return nil, err
	}

	info, err := statConfig(v, cfgPath)
	if err != nil {
		return nil, err
	}

	file, err := s.ParseUnitAST(
		ctx,
		v.FS,
		cfgPath,
		info,
		pkghclparse.NewParser(pc.parserOptions(l, v)...),
	)
	if err != nil {
		return nil, err
	}

	base, err := s.resolveBase(ctx, l, v, pc, file)
	if base == nil {
		return nil, err
	}

	return maps.Clone(base.evalVars), base.err()
}

// ResolveUnitQueueConfig returns the merged config of the file at cfgPath with the run parts left out, resolving the
// file as [Store.ResolveUnitConfig] does. Each call gets a config of its own.
//
// Returns a partial config with the error when part of the parse fails, and no config when the parse produced none.
func (s *Store) ResolveUnitQueueConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (*pkgconfig.TerragruntConfig, error) {
	_, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)

	return cfg, err
}

// ResolveRunConfig fetches the dependency outputs of the file at cfgPath and returns its merged config with the run
// parts, as [UnitConfig.ToV1] does, resolving the file as [Store.ResolveUnitConfig] does.
//
// Each call fetches the outputs again, and s retains neither them nor the config.
//
// Returns a partial config with the error when part of the run fails, and no config when the parse produced none.
func (s *Store) ResolveRunConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (*pkgconfig.TerragruntConfig, error) {
	c, err := s.ResolveUnitConfig(ctx, l, v, pc, cfgPath)
	if c == nil {
		return nil, err
	}

	return c.ToV1(ctx, l, v, pc)
}

// ReadTerragruntConfig reads the config file of run as [pkgconfig.ReadTerragruntConfig] does, resolving the file as
// [Store.ResolveRunConfig] does.
func (s *Store) ReadTerragruntConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	run *pkgconfig.ParsingContext,
) (*pkgconfig.TerragruntConfig, error) {
	l.Debugf(
		"Reading Terragrunt config file at %s",
		util.RelPathForLog(run.RootWorkingDir, run.TerragruntConfigPath, run.LogShowAbsPaths),
	)

	return s.ResolveRunConfig(ctx, l, v, NewParseContext(run), run.TerragruntConfigPath)
}

// resolvedFile is what a memoized resolution produces.
type resolvedFile struct {
	// c is the parse, nil when it failed before decoding the file.
	c *UnitConfig
	// cfg is the config of the parse, nil when it produced none.
	cfg *pkgconfig.TerragruntConfig
}

// resolveFile resolves the file at cfgPath one level deeper than pc, through s when pc is memoized.
// The parsed config is nil when the parse failed before decoding the file.
func (s *Store) resolveFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	pc, err := pc.incrementDepth()
	if err != nil {
		return nil, nil, err
	}

	if !pc.memoized {
		return s.resolveFileOnce(ctx, l, v, pc, cfgPath)
	}

	key := Key{Path: filepath.Clean(cfgPath), Context: pc.file.key, Stage: StageQueue}
	flight := pc.withOwnReads()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) (*resolvedFile, error) {
		c, cfg, err := s.resolveFileOnce(ctx, l, v, flight, cfgPath)
		NoteReads(ctx, flight.sinks.filesRead.Paths()...)

		if c != nil {
			NoteFiles(ctx, c.dependencyReads...)
		}

		return &resolvedFile{c: c, cfg: cfg}, err
	})

	for _, path := range res.Reads {
		pc.sinks.filesRead.Add(path)
	}

	if res.Value == nil || res.Value.c == nil {
		return nil, nil, err
	}

	c := res.Value.c
	cfg := res.Value.cfg

	if res.Hit {
		cfg, err = c.assemble(ctx, l, v, pc, keptQueue{})
	}

	return c, cfg, err
}

// resolveFileOnce resolves the file at cfgPath with pc, sharing only the file's syntax tree through s.
func (s *Store) resolveFileOnce(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	info, err := statConfig(v, cfgPath)
	if err != nil {
		return nil, nil, err
	}

	var (
		c   *UnitConfig
		cfg *pkgconfig.TerragruntConfig
	)

	err = pkgconfig.TraceParseConfigFile(
		ctx,
		l,
		cfgPath,
		pc.settings.WorkingDir,
		len(pc.settings.PartialParseDecodeList) > 0,
		pc.settings.PartialParseDecodeList,
		pc.file.include,
		false,
		func(childCtx context.Context, l log.Logger) error {
			parser := pkghclparse.NewParser(pc.parserOptions(l, v)...)

			file, parseErr := s.ParseUnitAST(childCtx, v.FS, cfgPath, info, parser)
			if parseErr != nil {
				return parseErr
			}

			base, parseErr := s.resolveBase(childCtx, l, v, pc, file)
			if base == nil {
				return parseErr
			}

			c = s.prepare(childCtx, l, v, pc, file, base)

			cfg, parseErr = s.assembleFirst(childCtx, l, v, pc, c)

			return parseErr
		},
	)

	return c, cfg, err
}

// resolveBase returns the base of file, through s when pc is memoized. A base that holds errors is not retained, and
// the next caller evaluates it again.
//
// Returns nil and the error where pkg/config returns no config.
func (s *Store) resolveBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
) (*unitBase, error) {
	if !pc.memoized {
		return s.prepareBase(ctx, l, v, pc, file)
	}

	key := Key{Path: filepath.Clean(file.ConfigPath), Context: pc.file.key, Stage: StageBase}
	flight := pc.withOwnReads()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) (*unitBase, error) {
		base, err := s.prepareBase(ctx, l, v, flight, file)
		NoteReads(ctx, flight.sinks.filesRead.Paths()...)

		if base == nil {
			return nil, err
		}

		return base, base.err()
	})

	for _, path := range res.Reads {
		pc.sinks.filesRead.Add(path)
	}

	if res.Value == nil {
		return nil, err
	}

	return res.Value, nil
}

// statConfig returns the metadata of the config file at cfgPath.
//
// Returns [pkgconfig.TerragruntConfigNotFoundError] when the file does not exist.
func statConfig(v *venv.Venv, cfgPath string) (fs.FileInfo, error) {
	info, err := v.FS.Stat(cfgPath)
	if err == nil {
		return info, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return nil, pkgconfig.TerragruntConfigNotFoundError{Path: cfgPath}
	}

	return nil, fmt.Errorf("failed to get file info: %w", err)
}

// assembleFirst runs the assembly of a parse, which resolves the files c merges, and keeps those files on c. Nothing
// else holds c yet.
func (s *Store) assembleFirst(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	c *UnitConfig,
) (*pkgconfig.TerragruntConfig, error) {
	kept := newKeptFiles(c.Includes)

	cfg, err := c.assemble(ctx, l, v, pc, includeResolver{store: s, kept: kept})
	c.kept = *kept

	return cfg, err
}

// unitBase is a file's state through its base blocks. It is shared between the callers of a memoized parse.
type unitBase struct {
	// evalErr is the error of building the eval context.
	evalErr error
	// locals is the evaluated locals block, nil when the base blocks failed to decode.
	locals *cty.Value
	// evalVars is the variables the file's blocks evaluate with, nil when the eval context failed to build.
	evalVars map[string]cty.Value
	// errs is the non-fatal errors of the stages before the eval context.
	errs []error
	// featureBlocks is the feature blocks the file declares, nil when the base blocks failed to decode.
	featureBlocks pkgconfig.FeatureFlags
	// scope is the file's state with its base applied.
	scope fileScope
}

// err returns every error of b, joined.
func (b *unitBase) err() error {
	return errors.Join(append(slices.Clone(b.errs), b.evalErr)...)
}

// firstErr returns the first error of b, which is where pkg/config stops the partial parse of an included file.
func (b *unitBase) firstErr() error {
	if len(b.errs) > 0 {
		return b.errs[0]
	}

	return b.evalErr
}

// prepareBase runs the stages of [pkgconfig.ParseConfig] through the base blocks.
//
// Returns an error and no base where pkg/config returns no config.
func (s *Store) prepareBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
) (*unitBase, error) {
	pctx := pc.parsingContext()

	if err := pkgconfig.DetectDeprecatedConfigurations(ctx, pctx, l, file); err != nil {
		return nil, err
	}

	if err := pkgconfig.ValidateExpansionSpelling(file); err != nil {
		return nil, err
	}

	if pkgconfig.TerraformSourceReferencesDependency(file) {
		return nil, pkgconfig.TerraformSourceReferencesDependencyError{ConfigPath: file.ConfigPath}
	}

	if err := pkgconfig.ValidateExcludeReferences(file); err != nil {
		return nil, err
	}

	include := absInclude(pc)

	pc = pc.withInclude(include)

	var errs []error

	pc, err := withFileIAMRole(ctx, l, v, pc, file)
	if err != nil {
		errs = append(errs, err)
	}

	pctx = pc.parsingContext()

	unitValues, err := pkgconfig.ReadValues(ctx, l, v, pctx, filepath.Dir(file.ConfigPath))
	if err != nil {
		return nil, err
	}

	pc = pc.withValues(unitValues)
	pctx = pc.parsingContext()

	exposed := s.exposedInclude(v, pc)

	baseBlocks, err := pkgconfig.DecodeBaseBlocks(
		ctx,
		l,
		v,
		pctx,
		file,
		include,
		s.includedFeatureFlags(v, pc),
		exposed,
	)
	if err != nil {
		l.Warnf("Errors decoding base blocks in %s: %v", file.ConfigPath, err)
		errs = append(errs, err)
	}

	base := &unitBase{errs: errs}

	if baseBlocks != nil {
		pc = pc.withBaseBlocks(baseBlocks)
		base.locals = baseBlocks.Locals
		base.featureBlocks = baseBlocks.FeatureFlagBlocks
	}

	base.evalVars, base.evalErr = pkgconfig.TerragruntEvalVariables(
		ctx,
		l,
		pc.parsingContext(),
		file.ConfigPath,
		exposed,
	)
	base.scope = pc.file

	return base, nil
}

// withFileIAMRole returns pc with the IAM role options file sets, as [pkgconfig.ParseConfig] resolves them. A partial
// parse keeps the role of pc, as [pkgconfig.PartialParseConfig] does.
//
// Returns pc unchanged with the error when the options fail to resolve.
func withFileIAMRole(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
) (*ParseContext, error) {
	if len(pc.settings.PartialParseDecodeList) > 0 {
		return pc, nil
	}

	iamRoleOptions, err := pkgconfig.ResolveIAMRoleOptions(ctx, l, v, pc.parsingContext(), file, pc.file.include)
	if err != nil {
		return pc, err
	}

	return pc.withIAMRole(iamRoleOptions), nil
}

// prepare runs the stages of [pkgconfig.ParseConfig] after base through the block decode. It decodes file's dependency
// blocks when file resolves its own dependencies, and every part when the run supplied the `dependency` value.
func (s *Store) prepare(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
	base *unitBase,
) *UnitConfig {
	pc = pc.withFile(&base.scope)
	errs := slices.Clone(base.errs)

	c := &UnitConfig{
		file:     file,
		Locals:   base.locals,
		evalVars: base.evalVars,
		evalErr:  base.evalErr,
		scope:    pc.file,
	}

	if includes := pc.file.includes; includes != nil {
		c.Includes = includes
	}

	if pc.file.decodedDeps == nil && !pc.file.inheritsDependencies {
		depPC := pc.withOwnReads()

		set, err := c.newDependencySet(ctx, l, v, depPC, file, s.includedDependencies(v, depPC))
		c.dependencyReads = depPC.sinks.filesRead.Paths()

		if err != nil {
			errs = append(errs, err)
		}

		if set != nil {
			c.deps = set
			c.DependencyConfigs = set.blocks
		}
	}

	if base.evalErr != nil {
		errs = append(errs, base.evalErr)
	}

	pctx := pc.parsingContext()
	evalCtx := c.evalContext(ctx, l, v, pctx)
	c.prepErrs = errs

	c.decodeErr = file.ApplyFileUpdate()
	if c.decodeErr == nil {
		dependencyAttrs := dependencyAttributes(file.Body)

		diags := gohcl.DecodeBody(withoutAttributes{Body: file.Body, hidden: dependencyAttrs}, evalCtx, c)
		c.dependencyAttrs = dependencyAttrs

		var queueDiags hcl.Diagnostics

		c.Queue, queueDiags = c.decodeQueue(evalCtx)
		c.Queue.Engine, c.queueUnknownDiags = c.decodeQueueEngine(evalCtx)

		if pctx.SkipOutputsResolution {
			c.queueUnknownDiags = append(c.queueUnknownDiags, c.decodeQueueAttributes(evalCtx)...)
		}

		c.decodeErr = file.HandleDiagnostics(append(diags, queueDiags...))
	}

	c.file = withoutParser(file)

	return c
}

// withoutParser returns file with no parser, for keeping in a stage's result. Using it before [pkghclparse.File.Rebind]
// panics.
func withoutParser(file *pkghclparse.File) *pkghclparse.File {
	return &pkghclparse.File{File: file.File, ConfigPath: file.ConfigPath}
}

// absInclude returns pc's include block with its path made absolute, in a copy.
func absInclude(pc *ParseContext) *pkgconfig.IncludeConfig {
	include := pc.file.include
	if include == nil || include.Path == "" || filepath.IsAbs(include.Path) {
		return include
	}

	abs := *include
	abs.Path = filepath.Clean(filepath.Join(filepath.Dir(pc.settings.TerragruntConfigPath), abs.Path))

	return &abs
}
