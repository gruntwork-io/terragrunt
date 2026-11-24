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

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResolveUnitConfig resolves the config file at cfgPath and the files it merges in, fetching no dependency outputs.
//
// Calls for the same file and context while one is running share its evaluation. A later call evaluates the file
// again, served the syntax trees and dependency scans s retained.
//
// The [UnitConfig] is shared with every call that shares its evaluation, and must not be changed.
//
// Returns the errors of the parse, which [UnitConfig.ResolveRunConfig] returns again with those of resolving the
// outputs. Returns nil when the parse produced no config.
func (s *Store) ResolveUnitConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*UnitConfig, error) {
	pc = pc.forStore(v)

	ctx, release := s.withReleaseScope(ctx)
	defer release()

	c, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)
	if cfg == nil {
		return nil, err
	}

	return c, err
}

// ResolveUnitBaseVariables returns the variables the file at cfgPath evaluates its blocks with: its unit values and
// the locals, feature, and include blocks [DecodeBaseBlocks] decodes. It stops before the dependency blocks
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
	pc *ParsingContext,
	cfgPath string,
) (map[string]cty.Value, error) {
	pc = pc.forStore(v)

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
		hclparse.NewParser(pc.ParserOptions(l, v)...),
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
// file as [Store.ResolveUnitConfig] does. Each call gets a config of its own. When pc has a decode list, the decode
// diagnostics it reports and writes are only those of the base blocks and of the sections the list names.
//
// Returns a partial config with the error when part of the parse fails, and no config when the parse produced none.
func (s *Store) ResolveUnitQueueConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*TerragruntConfig, error) {
	pc = pc.forStore(v)

	ctx, release := s.withReleaseScope(ctx)
	defer release()

	_, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)

	return cfg, err
}

// ResolvePartialConfig returns the merged config of the file at cfgPath with the run parts that a section of pc's
// decode list decodes and that read no `dependency`, resolving the file as [Store.ResolveUnitConfig] does. It fetches
// no outputs. The config also holds every queue part, but the decode diagnostics it reports and writes are only those
// of the base blocks and of the sections the decode list names.
//
// Returns [PartialParseReadsDependencyError], with the config, for each merged file with a requested run part that
// reads `dependency`, unless pc skips resolving outputs. Returns a partial config with the error when part of the
// parse fails, and no config when the parse produced none.
//
// Returns [TerraformSourceReferencesDependencyError], [ExcludeReferencesDependencyError], or a diagnostic for a merged
// file whose terraform source, exclude block, or version attributes read `dependency`, only when the decode list names
// [TerraformSource], [ExcludeBlock], or [TerragruntVersionConstraints] respectively.
func (s *Store) ResolvePartialConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*TerragruntConfig, error) {
	pc = pc.forStore(v)

	ctx, release := s.withReleaseScope(ctx)
	defer release()

	c, cfg, err := s.resolveFile(ctx, l, v, pc, cfgPath)

	decodeList := pc.PartialParseDecodeList
	if c == nil || !slices.ContainsFunc(decodeList, decodesRunParts) {
		return cfg, err
	}

	src := keptUnknownOutputs{decodeList: decodeList, reportedList: decodeList}

	var readers []error
	if !pc.SkipOutputsResolution {
		src.readers = &readers
	}

	cfg, err = c.assemble(ctx, l, v, s, pc, src, c.functions(ctx, l, v, s))
	if len(readers) == 0 {
		return cfg, err
	}

	return cfg, errors.Join(append([]error{err}, readers...)...)
}

// ResolveRunConfig fetches the dependency outputs of the file at cfgPath and returns its merged config with the run
// parts, as [UnitConfig.ResolveRunConfig] does, resolving the file as [Store.ResolveUnitConfig] does.
//
// Each call fetches the outputs again, and s retains neither them nor the config.
//
// Returns a partial config with the error when part of the run fails, and no config when the parse produced none.
func (s *Store) ResolveRunConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*TerragruntConfig, error) {
	pc = pc.forStore(v)

	c, err := s.ResolveUnitConfig(ctx, l, v, pc, cfgPath)
	if c == nil {
		return nil, err
	}

	return c.ResolveRunConfig(ctx, l, v, s, pc)
}

// ResolveRunConfigFromBytes resolves the config file at cfgPath as [Store.ResolveRunConfig] does, as if the file held
// content, e.g. an unsaved editor buffer. Every read of cfgPath sees content, including an include or
// read_terragrunt_config that comes back to it, and every other file is read from v. The directory of cfgPath need
// not exist.
func (s *Store) ResolveRunConfigFromBytes(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
	content []byte,
) (*TerragruntConfig, error) {
	layer := vfs.NewMemMapFS()
	if err := vfs.WriteFile(layer, cfgPath, content, configContentPerm); err != nil {
		return nil, err
	}

	return s.ResolveRunConfig(ctx, l, v.WithFS(vfs.NewOverlayFS(v.FS, layer)), pc, cfgPath)
}

// configContentPerm is the mode of the file [Store.ResolveRunConfigFromBytes] puts content in.
const configContentPerm = 0o644

// ReadTerragruntConfig reads the config file of run, resolving it as [Store.ResolveRunConfig] does.
func (s *Store) ReadTerragruntConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	run *ParsingContext,
) (*TerragruntConfig, error) {
	l.Debugf(
		"Reading Terragrunt config file at %s",
		util.RelPathForLog(run.RootWorkingDir, run.TerragruntConfigPath, run.LogShowAbsPaths),
	)

	return s.ResolveRunConfig(ctx, l, v, run, run.TerragruntConfigPath)
}

// resolvedFile is what a memoized resolution produces.
type resolvedFile struct {
	// c is the parse, nil when it failed before decoding the file.
	c *UnitConfig
	// cfg is the config of the parse, nil when it produced none.
	cfg *TerragruntConfig
}

// resolveFile resolves the file at cfgPath one level deeper than pc, through s when pc is memoized.
// The parsed config is nil when the parse failed before decoding the file.
func (s *Store) resolveFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*UnitConfig, *TerragruntConfig, error) {
	pc, err := pc.incrementDepth()
	if err != nil {
		return nil, nil, err
	}

	if !pc.memoized {
		return s.resolveFileOnce(ctx, l, v, pc, cfgPath)
	}

	key := Key{Path: filepath.Clean(cfgPath), Context: pc.key, Stage: StageQueue}
	flight := pc.withOwnReads()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) (*resolvedFile, error) {
		c, cfg, err := s.resolveFileOnce(ctx, l, v, flight, cfgPath)
		NoteReads(ctx, flight.FilesRead.Paths()...)

		if c != nil {
			NoteFiles(ctx, c.dependencyReads...)
		}

		return &resolvedFile{c: c, cfg: cfg}, err
	})

	for _, path := range res.Reads {
		pc.FilesRead.Add(path)
	}

	if res.Value == nil || res.Value.c == nil {
		return nil, nil, err
	}

	c := res.Value.c
	cfg := res.Value.cfg

	if res.Hit {
		cfg, err = c.assemble(ctx, l, v, s, pc, keptQueue{}, c.functions(ctx, l, v, s))
	}

	return c, cfg, err
}

// resolveFileOnce resolves the file at cfgPath with pc, sharing only the file's syntax tree through s.
func (s *Store) resolveFileOnce(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
) (*UnitConfig, *TerragruntConfig, error) {
	info, err := statConfig(v, cfgPath)
	if err != nil {
		return nil, nil, err
	}

	parse := func(ctx context.Context, parser *hclparse.Parser) (*hclparse.File, error) {
		return s.ParseUnitAST(ctx, v.FS, cfgPath, info, parser)
	}

	return s.resolveParsed(ctx, l, v, pc, cfgPath, parse)
}

// resolveParsed resolves the file at cfgPath with pc from the syntax tree that parse returns.
func (s *Store) resolveParsed(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	cfgPath string,
	parse func(ctx context.Context, parser *hclparse.Parser) (*hclparse.File, error),
) (*UnitConfig, *TerragruntConfig, error) {
	var (
		c   *UnitConfig
		cfg *TerragruntConfig
	)

	err := TraceParseConfigFile(
		ctx,
		l,
		cfgPath,
		pc.WorkingDir,
		len(pc.PartialParseDecodeList) > 0,
		pc.PartialParseDecodeList,
		pc.include,
		false,
		func(childCtx context.Context, l log.Logger) error {
			parser := hclparse.NewParser(pc.ParserOptions(l, v)...)

			file, parseErr := parse(childCtx, parser)
			if parseErr != nil {
				return parseErr
			}

			base, parseErr := s.resolveBase(childCtx, l, v, pc, file)
			if base == nil {
				return parseErr
			}

			fns := newParseFunctions(childCtx, l, v, s, file)
			c = s.prepare(childCtx, l, v, pc, file, base, fns)

			cfg, parseErr = s.assembleFirst(childCtx, l, v, pc, c, fns)

			return parseErr
		},
	)

	return c, cfg, err
}

// resolveBase returns the base of file, through s when pc is memoized. A base that holds errors is not retained, and
// the next caller evaluates it again.
//
// Returns nil and the error when [Store.prepareBase] returns no base.
func (s *Store) resolveBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	file *hclparse.File,
) (*unitBase, error) {
	if !pc.memoized {
		return s.prepareBase(ctx, l, v, pc, file)
	}

	key := Key{Path: filepath.Clean(file.ConfigPath), Context: pc.key, Stage: StageBase}
	flight := pc.withOwnReads()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) (*unitBase, error) {
		base, err := s.prepareBase(ctx, l, v, flight, file)
		NoteReads(ctx, flight.FilesRead.Paths()...)

		if base == nil {
			return nil, err
		}

		return base, base.err()
	})

	for _, path := range res.Reads {
		pc.FilesRead.Add(path)
	}

	if res.Value == nil {
		return nil, err
	}

	return res.Value, nil
}

// statConfig returns the metadata of the config file at cfgPath.
//
// Returns [TerragruntConfigNotFoundError] when the file does not exist.
func statConfig(v *venv.Venv, cfgPath string) (fs.FileInfo, error) {
	info, err := v.FS.Stat(cfgPath)
	if err == nil {
		return info, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return nil, TerragruntConfigNotFoundError{Path: cfgPath}
	}

	return nil, fmt.Errorf("failed to get file info: %w", err)
}

// assembleFirst runs the assembly of a parse, which resolves the files c merges, and keeps those files on c. Nothing
// else holds c yet.
func (s *Store) assembleFirst(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	c *UnitConfig,
	fns *parseFunctions,
) (*TerragruntConfig, error) {
	kept := newKeptFiles(c.Includes)

	cfg, err := c.assemble(ctx, l, v, s, pc, includeResolver{kept: kept}, fns)
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
	featureBlocks FeatureFlags
	// scope is the file's state with its base applied.
	scope ParsingContext
}

// err returns every error of b, joined.
func (b *unitBase) err() error {
	return errors.Join(append(slices.Clone(b.errs), b.evalErr)...)
}

// firstErr returns the first error of b, which stops reading the dependency and feature blocks of an included
// file.
func (b *unitBase) firstErr() error {
	if len(b.errs) > 0 {
		return b.errs[0]
	}

	return b.evalErr
}

// prepareBase checks file and evaluates it through its base blocks: its IAM role, unit values, locals, feature
// flags, and include blocks.
//
// Returns an error and no base when a check of file fails or its unit values fail to read.
func (s *Store) prepareBase(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	file *hclparse.File,
) (*unitBase, error) {
	pctx := pc.detach()

	if err := DetectDeprecatedConfigurations(ctx, pctx, l, file); err != nil {
		return nil, err
	}

	if err := ValidateExpansionSpelling(file); err != nil {
		return nil, err
	}

	if err := validateDependencyReferences(pc, file); err != nil {
		return nil, err
	}

	include := absInclude(pc)

	pc = pc.withInclude(include)

	pathValue, err := PathVariable(ctx, l, v, s, pctx, file, include)
	if err != nil {
		return nil, err
	}

	pc = pc.withPath(pathValue)

	var errs []error

	pc, err = s.withFileIAMRole(ctx, l, v, pc, file)
	if err != nil {
		errs = append(errs, err)
	}

	pctx = pc.detach()

	unitValues, err := ReadValues(ctx, l, v, s, pctx, file, RoleOf(file, include))
	if err != nil {
		return nil, err
	}

	pc = pc.withUnitValues(unitValues)
	pctx = pc.detach()

	exposed, exposedErr := s.exposedInclude(v, pc, file)

	baseBlocks, err := DecodeBaseBlocks(
		ctx,
		l,
		v,
		s,
		pctx,
		file,
		include,
		s.includedFeatureFlags(v, pc),
		exposed,
	)
	if err != nil {
		if len(pc.PartialParseDecodeList) == 0 {
			l.Warnf("Errors decoding base blocks in %s: %v", file.ConfigPath, err)
		}

		errs = append(errs, err)
	}

	base := &unitBase{errs: errs}

	if baseBlocks != nil {
		pc = pc.withBaseBlocks(baseBlocks)
		base.locals = baseBlocks.Locals
		base.featureBlocks = baseBlocks.FeatureFlagBlocks
	}

	base.evalVars, base.evalErr = TerragruntEvalVariables(
		ctx,
		l,
		pc.detach(),
		file.ConfigPath,
		exposed,
	)

	if len(pc.PartialParseDecodeList) > 0 {
		if err := exposedErr(); err != nil {
			base.errs = append(base.errs, err)
		}
	}

	base.scope = pc.scope()

	return base, nil
}

// validateDependencyReferences rejects each part of file that a parse with pc decodes and that reads `dependency`
// where no dependency has run: the terraform source, the exclude block, the version and module directory attributes.
//
// Returns [TerraformSourceReferencesDependencyError], [ExcludeReferencesDependencyError], or the diagnostics of
// [ValidateVersionAttributeReferences] for the first such part, or [ModuleDirReferencesDependencyError].
func validateDependencyReferences(pc *ParsingContext, file *hclparse.File) error {
	if pc.decodes(TerraformSource) && TerraformSourceReferencesDependency(file) {
		return TerraformSourceReferencesDependencyError{ConfigPath: file.ConfigPath}
	}

	if slices.Contains(pc.PartialParseDecodeList, moduleDirAttributes) {
		if err := moduleDirDependencyError(file); err != nil {
			return err
		}
	}

	if pc.decodes(ExcludeBlock) {
		if err := ValidateExcludeReferences(file); err != nil {
			return err
		}
	}

	if pc.decodes(TerragruntVersionConstraints) {
		return ValidateVersionAttributeReferences(file)
	}

	return nil
}

// withFileIAMRole returns pc with the IAM role options a full parse of file evaluates with: the iam_role and
// iam_web_identity_token of its config, merged with those of the files it merges, under the options passed on the
// CLI. A role ARN passed on the CLI wins outright. A partial parse keeps the role of pc.
// A partial parse that decodes [moduleDirAttributes] takes the IAM role options of file instead.
//
// The options come from a partial resolution of file through s, whose errors the full parse reports again.
//
// Returns pc unchanged with the error when that resolution produced no config.
func (s *Store) withFileIAMRole(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	file *hclparse.File,
) (*ParsingContext, error) {
	if !pc.decodes(moduleDirAttributes) {
		return pc, nil
	}

	pctx := pc.detach()

	if pctx.OriginalIAMRoleOptions.RoleARN != "" ||
		!mayMergeDeclaration(ctx, l, v, s, pctx, file, RoleOf(file, pc.include), iamRoleSchema) {
		return pc.withIAMRole(FileIAMRoleOptions(pctx, iam.RoleOptions{})), nil
	}

	_, cfg, err := s.resolveFile(ctx, l, v, pc.forTerragruntFlags(), file.ConfigPath)
	if cfg == nil {
		return pc, err
	}

	return pc.withIAMRole(FileIAMRoleOptions(pctx, cfg.GetIAMRoleOptions())), nil
}

// prepare decodes the blocks of file with base. It decodes file's dependency blocks when file resolves its own
// dependencies, and every part when the run supplied the `dependency` value.
func (s *Store) prepare(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParsingContext,
	file *hclparse.File,
	base *unitBase,
	fns *parseFunctions,
) *UnitConfig {
	pc = pc.withFile(&base.scope)
	errs := slices.Clone(base.errs)

	c := &UnitConfig{
		file:     file,
		Locals:   base.locals,
		evalVars: base.evalVars,
		evalErr:  base.evalErr,
		scope:    pc.scope(),
		written:  &writtenDiagnostics{},
	}

	if includes := pc.includes; includes != nil {
		c.Includes = includes
	}

	if pc.decodedDependencies() == nil && !pc.inheritsDependencies {
		depPC := pc.withOwnReads()

		set, err := c.newDependencySet(ctx, l, v, s, depPC, file, s.includedDependencies(v, depPC), fns)
		c.dependencyReads = depPC.FilesRead.Paths()

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

	evalCtx := c.evalContext(fns, pc.detach())
	c.prepErrs = errs

	c.decodeErr = file.ApplyFileUpdate()
	c.queueDecodeErr = c.decodeErr

	if c.decodeErr == nil {
		runAttrs := runAttributes(file.Body)

		hidden := map[string]*hcl.Attribute{}
		maps.Copy(hidden, runAttrs)
		maps.Copy(hidden, dependencyVersionAttributes(file.Body))

		diags := gohcl.DecodeBody(withoutAttributes{Body: file.Body, hidden: hidden}, evalCtx, c)
		c.runAttrs = runAttrs
		c.terraformRunPart = c.Terraform != nil && c.Terraform.readsDependency()
		c.errorsRunPart = c.Errors != nil && c.Errors.readsDependency()

		var queueDiags hcl.Diagnostics

		c.Queue, queueDiags = c.decodeQueue(evalCtx)
		c.Queue.Engine, c.queueEngineDiags = c.decodeQueueEngine(evalCtx)

		diags = append(diags, queueDiags...)
		c.decodeErr = file.FilterDiagnostics(diags)
		c.queueDecodeErr = c.decodeErr

		if diags.HasErrors() {
			c.queueDecodeErr = withoutDiagnostics(c.decodeErr, unsupportedContent(file.Body, c))
		}
	}

	c.file = file.WithoutParser()

	return c
}

// unsupportedContent returns the diagnostics that decoding body into c reports for the top-level arguments and blocks
// the schema of c does not name.
func unsupportedContent(body hcl.Body, c *UnitConfig) hcl.Diagnostics {
	schema, _ := gohcl.ImpliedBodySchema(c)

	_, all := body.Content(schema)
	_, _, partial := body.PartialContent(schema)

	return excludeDiagnostics(all, partial)
}

// absInclude returns pc's include block with its path made absolute, in a copy.
func absInclude(pc *ParsingContext) *IncludeConfig {
	include := pc.include
	if include == nil || include.Path == "" || filepath.IsAbs(include.Path) {
		return include
	}

	abs := *include
	abs.Path = filepath.Clean(filepath.Join(filepath.Dir(pc.TerragruntConfigPath), abs.Path))

	return &abs
}
