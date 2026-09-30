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

// ParseConfigFile parses the config file at cfgPath and the files it merges in, caching nothing and fetching no
// dependency outputs. It returns the errors of the parse, which [UnitConfig.ToV1] returns again with those of
// resolving the outputs, and nil when the parse produced no config.
func ParseConfigFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	cfgPath string,
) (*UnitConfig, error) {
	c, cfg, err := parseFile(ctx, l, v, p, pc, cfgPath)
	if cfg == nil {
		return nil, err
	}

	return c, err
}

// ParseConfig parses the config in file and the files it merges in, as [ParseConfigFile] does.
func ParseConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	p *hclparse.Store,
	pc *ParseContext,
	file *pkghclparse.File,
) (*UnitConfig, error) {
	c, err := prepare(ctx, l, v, pc, file)
	if err != nil {
		return nil, err
	}

	cfg, err := c.assemble(ctx, l, v, includeParser{store: p})
	if cfg == nil {
		return nil, err
	}

	return c, err
}

// parseFile parses and assembles the file at cfgPath one level deeper than pc. The parsed config is nil when the
// parse failed before decoding the file.
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

	if _, err := v.FS.Stat(cfgPath); err != nil {
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
			file, parseErr := pkghclparse.NewParser(pc.parserOptions(l, v)...).
				ParseFromFile(v.FS, cfgPath)
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

	baseBlocks, err := pkgconfig.DecodeBaseBlocks(ctx, l, v, pctx, file, include)
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
