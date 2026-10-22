package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ParseConfigFile parses the config file at cfgPath and the files it merges in, fetching no dependency outputs,
// and returns the parse beside the config of the parse alone, with the run parts left out, which discovery reads.
// The parse is memoized in p by file and context, so a second call for the same file and context shares the
// first's evaluation and gets a config built anew from it. It returns the errors of the parse, which
// [UnitConfig.ToV1] returns again with those of resolving the outputs, and no parse when the parse produced no
// config.
//
// A memoized parse reports the files it read to p only when pc's run records its reads, so [hclparse.Store.Invalidate]
// finds a parse by a file it read through a function such as file only for such a run. It finds a parse by the
// config files it parsed either way.
func ParseConfigFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	c, cfg, err := parseFile(ctx, l, v, p, pc, cfgPath)
	if cfg == nil {
		return nil, nil, err
	}

	return c, cfg, err
}

// ParseConfig parses the config in file and the files it merges in, as [ParseConfigFile] does. The parse of file
// itself is not memoized, since file need not come from the path it names.
func ParseConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	file *pkghclparse.File,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	c, err := prepare(ctx, l, v, pc, file)
	if err != nil {
		return nil, nil, err
	}

	cfg, err := c.assemble(ctx, l, v, includeParser{store: p})
	if cfg == nil {
		return nil, nil, err
	}

	return c, cfg, err
}

// parsedFile is what a memoized parse produces.
type parsedFile struct {
	// c is the parse, nil when it failed before decoding the file.
	c *UnitConfig
	// cfg is the config of the parse, nil when it produced none.
	cfg *pkgconfig.TerragruntConfig
}

// parseFile parses and assembles the file at cfgPath one level deeper than pc, through p when pc is memoized.
// The parsed config is nil when the parse failed before decoding the file.
func parseFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	pc, err := pc.incrementDepth()
	if err != nil {
		return nil, nil, err
	}

	if !pc.memoized {
		return parseFileOnce(ctx, l, v, p, pc, cfgPath)
	}

	key := hclparse.Key{Path: filepath.Clean(cfgPath), Context: pc.file.key, Stage: hclparse.StageQueue}
	flight := pc.forFlight()

	res, err := p.Resolve(ctx, key, func(ctx context.Context) (*parsedFile, error) {
		c, cfg, err := parseFileOnce(ctx, l, v, p, flight, cfgPath)
		hclparse.NoteReads(ctx, flight.run.FilesRead.Paths()...)

		return &parsedFile{c: c, cfg: cfg}, err
	})

	for _, path := range res.Reads {
		pc.run.FilesRead.Add(path)
	}

	if res.Value == nil || res.Value.c == nil {
		return nil, nil, err
	}

	c := res.Value.c.rebind(l, v, pc.run)
	cfg := res.Value.cfg

	if res.Hit && err == nil {
		cfg, err = c.assemble(ctx, l, v, keptQueue{})
	}

	return c, cfg, err
}

// parseFileOnce parses and assembles the file at cfgPath with pc, sharing only the file's syntax tree through p.
func parseFileOnce(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, *pkgconfig.TerragruntConfig, error) {
	info, err := v.FS.Stat(cfgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, pkgconfig.TerragruntConfigNotFoundError{Path: cfgPath}
		}

		return nil, nil, fmt.Errorf("failed to get file info: %w", err)
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

			file, parseErr := p.AST(childCtx, v.FS, cfgPath, info, parser)
			if parseErr != nil {
				return parseErr
			}

			c, parseErr = prepare(childCtx, l, v, pc, file)
			if parseErr != nil {
				return parseErr
			}

			cfg, parseErr = c.assemble(childCtx, l, v, includeParser{store: p})

			return parseErr
		},
	)

	return c, cfg, err
}

// rebind returns a copy of c reading run's settings, whose FilesRead receives what [UnitConfig.ToV1] reads and
// whose logger and writers receive the diagnostics. Its file and kept files are rebound the same way. c is never
// changed, so the memoized parse serves every caller.
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

	baseBlocks, err := pkgconfig.DecodeBaseBlocks(ctx, l, v, pctx, file, include, pkgconfig.IncludeParseFeatureFlags)
	if err != nil {
		l.Warnf("Errors decoding base blocks in %s: %v", file.ConfigPath, err)
		errs = append(errs, err)
	}

	c := &UnitConfig{file: file}

	if baseBlocks != nil {
		pc = pc.withBaseBlocks(baseBlocks)
		c.Locals = baseBlocks.Locals
	}

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

	pctx = pc.parsingContext()

	evalCtx, err := pkgconfig.CreateTerragruntEvalContext(ctx, l, v, pctx, file.ConfigPath)
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
