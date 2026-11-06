package config

import (
	"context"
	"fmt"
	"maps"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// withDeclaredOutputs adds the mock values and mock_strategy the unit at config_path declares to a copy of dep,
// with dep's own settings winning, and returns that copy with the declared contract. It returns dep itself and no
// contract when the experiment is off or dep reads no outputs block.
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

// DeclaredOutputs returns the outputs block of the unit dep reads, or nil when dep is disabled, its config_path is
// not a string or names no unit config, or that unit declares no outputs block. A unit config without the block
// gives the outputs-block tip, once per run.
//
// Returns [DependencyBlockError] naming dep, wrapping [DeclaredOutputsReadError] when the unit config cannot be
// read and [UnknownMockValueError] when a mock value it declares is not known.
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

// readDeclaredOutputs returns a copy of the outputs block of the unit config at targetConfig, merged with those of
// the configs it includes, or nil when it declares none. Each unit config is parsed once per run unless its size
// or modification time changes; later reads, by any dependent, come from the cache in the run context. The cache
// key leaves the parser settings out, since every read parses with the settings of parseDeclaredOutputs.
//
// Returns [DeclaredOutputsReadError] when the config cannot be read, and [UnknownMockValueError] when a mock value
// it declares is not known.
func readDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	targetConfig string,
) (*OutputsConfig, error) {
	fileInfo, err := v.FS.Stat(targetConfig)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
	}

	declaredCache := cache.ContextCache[*OutputsConfig](ctx, DeclaredOutputsCacheContextKey)
	cacheKey := fmt.Sprintf(
		"configPath-%s-modTime-%d-size-%d",
		targetConfig,
		fileInfo.ModTime().UnixMicro(),
		fileInfo.Size(),
	)

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

// parseDeclaredOutputs decodes only the outputs block of the unit config at targetConfig into the result, with
// the locals and includes it needs to evaluate the mock values, so its dependency outputs stay unresolved and no
// state is read. A diagnostic the parse raises is an error, and a mock value that reads a dependency output is
// rejected as unknown.
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
	// A collecting handler would swallow the diagnostic and read the block as absent.
	targetPctx.Parser.DiagnosticsHandler = nil

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

// checkDeclaredOutputs checks that stateOutputs, read from the state of the unit at targetConfig, holds every
// output that declared, the unit's outputs block, names, or that the merge strategy of dep fills the output from
// its mock value. Render commands tolerate unresolved outputs, so they are not checked.
//
// Returns [DeclaredOutputMissingError] for the first declared output that is missing.
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

		return DeclaredOutputMissingError{
			DependencyName: dep.Name,
			TargetConfig:   targetConfig,
			CurrentConfig:  pctx.TerragruntConfigPath,
			Output:         name,
		}
	}

	return nil
}

// mergeDeclaredMockValues returns the mock values a dependent reads: each key of overrides, the dependent's
// mock_outputs, wins over the same key of declared, the mock_value attributes of the unit's outputs block. Either
// side may be nil. An override that is not a known object or map replaces declared outright. The marks of a
// sensitive() mock_outputs survive the merge.
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

// mockValueMap returns the entries of mocks, a mock_outputs value, by key, each carrying the marks of mocks as a
// whole, so the entries of a sensitive() object stay sensitive on their own. It reports false when mocks is not a
// known object or map.
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
