package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// withDeclaredOutputs returns dep, or a copy with the declared mock values under dep's own settings and the contract.
func (dep *Dependency) withDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (*Dependency, *OutputsConfig, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return dep, nil, nil
	}

	declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	if err != nil {
		return nil, nil, err
	}

	if declared == nil {
		return dep, nil, nil
	}

	effective := *dep
	effective.MockOutputs = mergeDeclaredMockValues(declared.MockValues(), dep.MockOutputs)

	strategy := declared.MockStrategy
	if strategy == nil {
		return &effective, declared, nil
	}

	if dep.MockOutputsAllowedTerraformCommands == nil && strategy.AllowedCommands != nil {
		effective.MockOutputsAllowedTerraformCommands = strategy.AllowedCommands
	}

	if dep.MockOutputsMergeStrategyWithState == nil && dep.MockOutputsMergeWithState == nil &&
		strategy.MergeWithState != nil {
		effective.MockOutputsMergeStrategyWithState = strategy.MergeWithState
	}

	return &effective, declared, nil
}

// DeclaredOutputs returns the outputs block of the unit dep reads, nil when there is none, or [DependencyBlockError].
func (dep *Dependency) DeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (*OutputsConfig, error) {
	if dep.isDisabled() {
		return nil, nil
	}

	configPath, ok := dep.configPathString()
	if !ok {
		return nil, nil
	}

	targetConfig := getCleanedTargetConfigPath(v.FS, configPath, pctx.TerragruntConfigPath)
	if !vfs.Exists(v.FS, targetConfig) {
		return nil, nil
	}

	if stackFile, ok := resolveStackFilePath(configPath, targetConfig); ok && vfs.Exists(v.FS, stackFile) {
		return nil, nil
	}

	declared, err := readDeclaredOutputs(ctx, l, v, pctx, targetConfig)
	if err != nil {
		return nil, DependencyBlockError{Name: dep.Name, ConfigPath: pctx.TerragruntConfigPath, Err: err}
	}

	if declared == nil && pctx.OutputsBlockTip != nil {
		pctx.OutputsBlockTip(l, targetConfig)
	}

	return declared, nil
}

// readDeclaredOutputs returns a copy of the outputs block of the unit config at targetConfig, cached per run, or nil.
func readDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	targetConfig string,
) (*OutputsConfig, error) {
	declaredCache := cache.ContextCache[*declaredOutputsResult](ctx, DeclaredOutputsCacheContextKey)

	if result, found := declaredCache.Get(ctx, targetConfig); found && result.fresh(v.FS) {
		result.replayFilesRead(pctx)

		return result.declared.Clone(), nil
	}

	key := evaluatedOutputsKey(v, pctx, targetConfig)

	if result, found := declaredCache.Get(ctx, key); found && result.fresh(v.FS) {
		result.replayFilesRead(pctx)

		return result.declared.Clone(), nil
	}

	parsePctx := pctx.Clone()
	parsePctx.FilesRead = NewFilesRead()
	parsePctx.UsePartialParseConfigCache = false

	readAt := time.Now()

	declared, err := parseDeclaredOutputs(ctx, l, v, parsePctx, targetConfig)

	result := &declaredOutputsResult{
		declared: declared,
		files:    map[string]fileStamp{targetConfig: stampFile(v.FS, targetConfig, readAt)},
	}

	for _, path := range parsePctx.FilesRead.Paths() {
		result.files[path] = stampFile(v.FS, path, readAt)
	}

	result.replayFilesRead(pctx)

	if err != nil {
		return nil, err
	}

	declaredCache.Put(ctx, key, result)

	return declared.Clone(), nil
}

// evaluatedOutputsKey returns the cache key of an evaluated outputs block: its config and what it reads of the reader.
func evaluatedOutputsKey(v *venv.Venv, pctx *ParsingContext, targetConfig string) string {
	key := NewReadTerragruntConfigKey(v, pctx)
	key.ConfigPath = targetConfig
	key.DecodeList = nil

	return fmt.Sprintf("%#v", key)
}

type declaredOutputsResult struct {
	declared *OutputsConfig
	files    map[string]fileStamp
}

// fresh reports whether every file the read took matches its stamp.
func (r *declaredOutputsResult) fresh(fsys vfs.FS) bool {
	for path, stamp := range r.files {
		if !stamp.matches(fsys, path) {
			return false
		}
	}

	return true
}

// replayFilesRead records every file the read took in the files pctx reads.
func (r *declaredOutputsResult) replayFilesRead(pctx *ParsingContext) {
	for _, path := range slices.Sorted(maps.Keys(r.files)) {
		pctx.FilesRead.Add(path)
	}
}

// parseDeclaredOutputs decodes only the outputs block of the unit config at targetConfig, with its locals and includes.
func parseDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	targetConfig string,
) (*OutputsConfig, error) {
	l, targetPctx, err := pctx.WithDependencyConfigPath(l, targetConfig)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	targetPctx = targetPctx.WithDecodeList(OutputsBlock).WithDiagnosticsSuppressed().WithSkipOutputsResolution()
	targetPctx.DecodedDependencies = nil
	targetPctx.Parser.DiagnosticsHandler = nil
	targetPctx.Parser.IgnoreDiagnostics = false
	targetPctx.Parser.HaltOnErrorOnlyInBlocks = nil

	source, err := vfs.ReadFile(v.FS, targetConfig)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	file, err := targetPctx.NewParser(l, v).ParseFromString(string(source), targetConfig)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	targetPctx.IAMRoleOptions, err = ResolveIAMRoleOptions(ctx, l, v, targetPctx, file, nil)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	cfg, err := PartialParseConfig(ctx, l, v, targetPctx, file, nil)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	if cfg.Outputs == nil {
		return nil, nil
	}

	for _, output := range cfg.Outputs.Outputs {
		if output.MockValue != nil && !output.MockValue.IsWhollyKnown() {
			return nil, UnknownMockValueError{ConfigPath: targetConfig, Output: output.Name}
		}
	}

	return cfg.Outputs, nil
}

// settleDeclaredOutputs caches the outputs block of the unit config at targetConfig when its files give it unevaluated.
func settleDeclaredOutputs(
	ctx context.Context,
	v *venv.Venv,
	pctx *ParsingContext,
	targetConfig string,
	includes IncludeConfigsMap,
) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return
	}

	declaredCache := cache.ContextCache[*declaredOutputsResult](ctx, DeclaredOutputsCacheContextKey)
	if result, found := declaredCache.Get(ctx, targetConfig); found && result.fresh(v.FS) {
		return
	}

	if result := declaredOutputsInFiles(ctx, v, pctx, targetConfig, includes); result != nil {
		declaredCache.Put(ctx, targetConfig, result)
	}
}

// declaredOutputsInFiles returns the outputs block only targetConfig can declare, absent or all constants, else nil.
func declaredOutputsInFiles(
	ctx context.Context,
	v *venv.Venv,
	pctx *ParsingContext,
	targetConfig string,
	includes IncludeConfigsMap,
) *declaredOutputsResult {
	readAt := time.Now()

	info, err := v.FS.Stat(targetConfig)
	if err != nil {
		return nil
	}

	hclCache := cache.ContextCache[*CachedHCLFile](ctx, HclCacheContextKey)

	file := cachedHCLFile(ctx, v.FS, hclCache, partialParseCacheKey(targetConfig, info), targetConfig)
	if file == nil || !file.ParsedClean() {
		return nil
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(topLevelBlocks(body, MetadataInclude)) != len(includes) {
		return nil
	}

	files := map[string]fileStamp{targetConfig: stampFile(v.FS, targetConfig, readAt)}

	for _, include := range includes {
		includePath := include.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(targetConfig), includePath)
		}

		if includeDeclaresOutputs(ctx, v, pctx, includePath) {
			return nil
		}

		files[includePath] = stampFile(v.FS, includePath, readAt)
	}

	if autoIncludeBeside(v.FS, pctx, targetConfig) {
		return nil
	}

	blocks := topLevelBlocks(body, MetadataOutputs)
	if len(blocks) == 0 {
		return &declaredOutputsResult{files: files}
	}

	if len(blocks) > 1 || len(blocks[0].Labels) > 0 {
		return nil
	}

	declared := &OutputsConfig{}
	if diags := gohcl.DecodeBody(blocks[0].Body, &hcl.EvalContext{}, declared); len(diags) > 0 {
		return nil
	}

	if err := declared.Validate(pctx.Experiments, targetConfig); err != nil {
		return nil
	}

	return &declaredOutputsResult{declared: declared, files: files}
}

// includeDeclaresOutputs reports whether the included config at path can add an outputs block, or true when unsure.
func includeDeclaresOutputs(ctx context.Context, v *venv.Venv, pctx *ParsingContext, path string) bool {
	if autoIncludeBeside(v.FS, pctx, path) {
		return true
	}

	info, err := v.FS.Stat(path)
	if err != nil {
		return true
	}

	hclCache := cache.ContextCache[*CachedHCLFile](ctx, HclCacheContextKey)

	file := cachedHCLFile(ctx, v.FS, hclCache, partialParseCacheKey(path, info), path)
	if file == nil {
		return true
	}

	body, ok := file.Body.(*hclsyntax.Body)

	return !ok || len(topLevelBlocks(body, MetadataOutputs)) > 0
}

// autoIncludeBeside reports whether a parse of the config at path merges an autoinclude file beside it.
func autoIncludeBeside(fsys vfs.FS, pctx *ParsingContext, path string) bool {
	autoInclude, ok := siblingAutoIncludePath(pctx, path)

	return ok && vfs.Exists(fsys, autoInclude)
}

// topLevelBlocks returns the top-level blocks of body that have the type blockType.
func topLevelBlocks(body *hclsyntax.Body, blockType string) []*hclsyntax.Block {
	var blocks []*hclsyntax.Block

	for _, block := range body.Blocks {
		if block.Type == blockType {
			blocks = append(blocks, block)
		}
	}

	return blocks
}

// checkDeclaredOutputs returns a [DeclaredOutputMissingError] for each declared output missing from stateOutputs.
func checkDeclaredOutputs(
	pctx *ParsingContext,
	dep *Dependency,
	declared *OutputsConfig,
	targetConfig string,
	stateOutputs map[string]cty.Value,
) error {
	if declared == nil || isRenderJSONCommand(pctx) || isRenderCommand(pctx) {
		return nil
	}

	mocks := stateFillingMocks(pctx, dep)
	names := declared.Names()
	errs := make([]error, 0, len(names))

	for _, name := range names {
		if _, ok := stateOutputs[name]; ok {
			continue
		}

		if _, ok := mocks[name]; ok {
			continue
		}

		errs = append(errs, DeclaredOutputMissingError{TargetConfig: targetConfig, Output: name})
	}

	return errors.Join(errs...)
}

// stateFillingMocks returns the mock values that stand in for outputs the state lacks, none unless dep merges them.
func stateFillingMocks(pctx *ParsingContext, dep *Dependency) map[string]cty.Value {
	if !dep.shouldMergeMockOutputsWithState(pctx) {
		return nil
	}

	values, ok := mockValueMap(dep.MockOutputs)
	if !ok {
		return nil
	}

	return values
}

// mergeDeclaredMockValues returns the mock values a dependent reads, each key of overrides winning over declared.
func mergeDeclaredMockValues(declared, overrides *cty.Value) *cty.Value {
	if overrides == nil {
		return declared
	}

	if declared == nil {
		return overrides
	}

	overrideValues, ok := mockValueMap(overrides)
	if !ok {
		return overrides
	}

	merged := make(map[string]cty.Value, declared.LengthInt()+len(overrideValues))
	maps.Copy(merged, declared.AsValueMap())
	maps.Copy(merged, overrideValues)

	object := cty.ObjectVal(merged)

	return &object
}

// mockValueMap returns the entries of a known object or map by key, each carrying its marks, or false otherwise.
func mockValueMap(mocks *cty.Value) (map[string]cty.Value, bool) {
	if mocks == nil || mocks.IsNull() || !mocks.IsKnown() {
		return nil, false
	}

	if mocksType := mocks.Type(); !mocksType.IsObjectType() && !mocksType.IsMapType() {
		return nil, false
	}

	unmarked, marks := mocks.Unmark()
	values := unmarked.AsValueMap()

	if len(marks) == 0 {
		return values, true
	}

	for key, value := range values {
		values[key] = value.WithMarks(marks)
	}

	return values, true
}
