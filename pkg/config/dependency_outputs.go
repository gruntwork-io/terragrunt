package config

import (
	"context"
	"fmt"
	"maps"
	"time"

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
	readAt := time.Now()

	fileInfo, err := v.FS.Stat(targetConfig)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	cacheKey := fmt.Sprintf(
		"configPath-%s-modTime-%d-size-%d",
		targetConfig,
		fileInfo.ModTime().UnixMicro(),
		fileInfo.Size(),
	)

	if isRacyModTime(fileInfo.ModTime(), readAt) {
		digest, err := digestFile(v.FS, targetConfig)
		if err != nil {
			return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
		}

		cacheKey += fmt.Sprintf("-digest-%d", digest)
	}

	declaredCache := cache.ContextCache[*OutputsConfig](ctx, DeclaredOutputsCacheContextKey)

	if declared, found := declaredCache.Get(ctx, cacheKey); found {
		return declared.Clone(), nil
	}

	declared, err := parseDeclaredOutputs(ctx, l, v, pctx, targetConfig)
	if err != nil {
		return nil, err
	}

	declaredCache.Put(ctx, cacheKey, declared)

	return declared.Clone(), nil
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

	cfg, err := PartialParseConfigString(ctx, l, v, targetPctx, targetConfig, string(source), nil)
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

// checkDeclaredOutputs returns [DeclaredOutputMissingError] for the first declared output missing from stateOutputs.
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

	var mocks map[string]cty.Value

	if dep.shouldMergeMockOutputsWithState(pctx) {
		if values, ok := mockValueMap(dep.MockOutputs); ok {
			mocks = values
		}
	}

	for _, name := range declared.Names() {
		if _, ok := stateOutputs[name]; ok {
			continue
		}

		if _, ok := mocks[name]; ok {
			continue
		}

		return DeclaredOutputMissingError{TargetConfig: targetConfig, Output: name}
	}

	return nil
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
