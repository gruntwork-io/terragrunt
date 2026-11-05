package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"

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
// The file's syntax tree is shared through s, and the variables are evaluated anew on each call.
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

	base, err := prepareBase(ctx, l, v, pc, file)
	if err != nil {
		return nil, err
	}

	evalCtx, err := pkgconfig.CreateTerragruntEvalContext(
		ctx,
		l,
		v,
		base.pc.parsingContext(),
		file.ConfigPath,
	)
	errs := errors.Join(append(base.errs, err)...)

	if evalCtx == nil {
		return nil, errs
	}

	return evalCtx.Variables, errs
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

	return c.ToV1(ctx, l, v)
}

// ResolveUnitParsedFile resolves the config in file and the files it merges in, as [Store.ResolveUnitConfig] does. The
// result for file itself is not memoized.
func (s *Store) ResolveUnitParsedFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
) (*UnitConfig, error) {
	c, err := prepare(ctx, l, v, pc, file)
	if err != nil {
		return nil, err
	}

	cfg, err := c.assemble(ctx, l, v, includeResolver{store: s})
	if cfg == nil {
		return nil, err
	}

	return c, err
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
	flight := pc.forFlight()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) (*resolvedFile, error) {
		c, cfg, err := s.resolveFileOnce(ctx, l, v, flight, cfgPath)
		NoteReads(ctx, flight.run.FilesRead.Paths()...)

		return &resolvedFile{c: c, cfg: cfg}, err
	})

	for _, path := range res.Reads {
		pc.run.FilesRead.Add(path)
	}

	if res.Value == nil || res.Value.c == nil {
		return nil, nil, err
	}

	c := res.Value.c.rebind(l, v, pc.run)
	cfg := res.Value.cfg

	if res.Hit {
		cfg, err = c.assemble(ctx, l, v, keptQueue{})
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
		pc.run.WorkingDir,
		len(pc.run.PartialParseDecodeList) > 0,
		pc.run.PartialParseDecodeList,
		pc.file.include,
		false,
		func(childCtx context.Context, l log.Logger) error {
			parser := pkghclparse.NewParser(pc.parserOptions(l, v)...)

			file, parseErr := s.ParseUnitAST(childCtx, v.FS, cfgPath, info, parser)
			if parseErr != nil {
				return parseErr
			}

			c, parseErr = prepare(childCtx, l, v, pc, file)
			if parseErr != nil {
				return parseErr
			}

			cfg, parseErr = c.assemble(childCtx, l, v, includeResolver{store: s})

			return parseErr
		},
	)

	return c, cfg, err
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

// rebind returns a copy of c reading run's settings, whose FilesRead receives what [UnitConfig.ToV1] reads and
// whose logger and writers receive the diagnostics. Its file and kept files are rebound the same way. c is never
// changed.
func (c *UnitConfig) rebind(l log.Logger, v *venv.Venv, run *pkgconfig.ParsingContext) *UnitConfig {
	r := *c
	r.pc = c.pc.withRun(run)
	r.file = c.file.Rebind(pkghclparse.NewParser(r.pc.parserOptions(l, v)...))

	if c.deps != nil {
		deps := *c.deps
		deps.file = r.file
		r.deps = &deps
	}

	if c.autoInclude != nil {
		r.autoInclude = c.autoInclude.rebind(l, v, run)
	}

	if c.includedFiles != nil {
		r.includedFiles = make([]*includedFile, len(c.includedFiles))

		for i, included := range c.includedFiles {
			if included != nil {
				r.includedFiles[i] = included.rebind(l, v, run)
			}
		}
	}

	return &r
}

// unitBase is a file's state through its base blocks.
type unitBase struct {
	// pc is the file's parse context with its base state applied.
	pc *ParseContext
	// locals is the evaluated locals block, nil when the base blocks failed to decode.
	locals *cty.Value
	// errs is the non-fatal errors of the stages so far.
	errs []error
}

// prepareBase runs the stages of [pkgconfig.ParseConfig] through the base blocks.
//
// Returns an error and no base where pkg/config returns no config.
func prepareBase(
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
	pctx = pc.parsingContext()

	var errs []error

	iamRoleOptions, err := pkgconfig.ResolveIAMRoleOptions(ctx, l, v, pctx, file, include)
	if err != nil {
		errs = append(errs, err)
	}

	if err == nil {
		pc = pc.withIAMRole(iamRoleOptions)
		pctx.IAMRoleOptions = iamRoleOptions
	}

	unitValues, err := pkgconfig.ReadValues(ctx, l, v, pctx, filepath.Dir(file.ConfigPath))
	if err != nil {
		return nil, err
	}

	pc = pc.withValues(unitValues)
	pctx = pc.parsingContext()

	baseBlocks, err := pkgconfig.DecodeBaseBlocks(
		ctx,
		l,
		v,
		pctx,
		file,
		include,
		pkgconfig.IncludeParseFeatureFlags,
	)
	if err != nil {
		l.Warnf("Errors decoding base blocks in %s: %v", file.ConfigPath, err)
		errs = append(errs, err)
	}

	base := &unitBase{pc: pc, errs: errs}

	if baseBlocks != nil {
		base.pc = pc.withBaseBlocks(baseBlocks)
		base.locals = baseBlocks.Locals
	}

	return base, nil
}

// prepare runs the stages of [pkgconfig.ParseConfig] through the block decode. It decodes file's dependency
// blocks when file resolves its own dependencies, and every part when the run supplied the `dependency` value.
//
// Returns an error and no config where pkg/config returns no config.
func prepare(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
	file *pkghclparse.File,
) (*UnitConfig, error) {
	base, err := prepareBase(ctx, l, v, pc, file)
	if err != nil {
		return nil, err
	}

	pc = base.pc
	errs := base.errs

	c := &UnitConfig{file: file, Locals: base.locals}

	if includes := pc.file.includes; includes != nil {
		c.Includes = includes
		c.includedFiles = make([]*includedFile, len(includes.List))
	}

	if pc.file.decodedDeps == nil && !pc.file.inheritsDependencies {
		set, err := newDependencySet(ctx, l, v, pc.parsingContext(), file)
		if err != nil {
			errs = append(errs, err)
		}

		if set != nil {
			c.deps = set
			c.DependencyConfigs = set.blocks
		}
	}

	evalCtx, err := pkgconfig.CreateTerragruntEvalContext(
		ctx,
		l,
		v,
		pc.parsingContext(),
		file.ConfigPath,
	)
	if err != nil {
		errs = append(errs, err)
	}

	if evalCtx != nil {
		c.evalVars = evalCtx.Variables
	}

	c.pc = pc
	c.prepErrs = errs

	c.decodeErr = file.ApplyFileUpdate()
	if c.decodeErr == nil {
		diags := gohcl.DecodeBody(file.Body, evalCtx, c)

		var queueDiags hcl.Diagnostics

		c.Queue, queueDiags = c.decodeQueue(evalCtx)
		c.decodeErr = file.HandleDiagnostics(append(diags, queueDiags...))
	}

	return c, nil
}

// absInclude returns pc's include block with its path made absolute, in a copy.
func absInclude(pc *ParseContext) *pkgconfig.IncludeConfig {
	include := pc.file.include
	if include == nil || include.Path == "" || filepath.IsAbs(include.Path) {
		return include
	}

	abs := *include
	abs.Path = filepath.Clean(filepath.Join(filepath.Dir(pc.run.TerragruntConfigPath), abs.Path))

	return &abs
}
