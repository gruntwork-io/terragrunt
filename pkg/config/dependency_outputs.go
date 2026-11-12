package config

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type declaredDependency struct {
	dep     *Dependency
	outputs *OutputsConfig
}

// withDeclaredOutputs returns dep with the mock values and mock strategy that its unit declares filled in.
func (dep *Dependency) withDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (declaredDependency, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return declaredDependency{dep: dep}, nil
	}

	declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	if err != nil {
		return declaredDependency{}, err
	}

	if declared == nil {
		return declaredDependency{dep: dep}, nil
	}

	effective := *dep
	effective.MockOutputs = mergeDeclaredMockValues(declared.MockValues(), dep.MockOutputs)

	strategy := declared.MockStrategy
	if strategy == nil {
		return declaredDependency{dep: &effective, outputs: declared}, nil
	}

	if dep.MockOutputsAllowedTerraformCommands == nil && strategy.AllowedCommands != nil {
		effective.MockOutputsAllowedTerraformCommands = strategy.AllowedCommands
	}

	if dep.MockOutputsMergeStrategyWithState == nil && dep.MockOutputsMergeWithState == nil &&
		strategy.MergeWithState != nil {
		effective.MockOutputsMergeStrategyWithState = strategy.MergeWithState
	}

	return declaredDependency{dep: &effective, outputs: declared}, nil
}

// DeclaredOutputs returns the outputs block of the unit dep reads, nil when there is none, or [DependencyBlockError].
func (dep *Dependency) DeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (*OutputsConfig, error) {
	target, err := dep.readTarget(ctx, l, v, pctx)
	if err != nil {
		return nil, DependencyBlockError{Name: dep.Name, ConfigPath: pctx.TerragruntConfigPath, Err: err}
	}

	return target.outputs.Clone(), nil
}

type dependencyTarget struct {
	outputs    *OutputsConfig
	configPath string
}

// readTarget returns the unit config dep reads and its outputs block, or the zero value when no unit config parses.
func (dep *Dependency) readTarget(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (dependencyTarget, error) {
	configPath, ok := dep.unitTarget(v.FS, pctx)
	if !ok {
		return dependencyTarget{}, nil
	}

	single := TerragruntDependency{Dependencies: Dependencies{*dep}}
	if _, err := decodeDependencies(ctx, l, v, pctx, single); err != nil {
		return dependencyTarget{}, err
	}

	depCache := cache.ContextCache[*dependencyOutputCache](ctx, DependencyOutputCacheContextKey)

	parsed, found := depCache.Get(ctx, filepath.Join(pctx.WorkingDir, configPath))
	if !found {
		return dependencyTarget{}, nil
	}

	target := dependencyTarget{configPath: configPath, outputs: parsed.Outputs}
	if parsed.Outputs == nil {
		return target, nil
	}

	for _, output := range parsed.Outputs.Outputs {
		if output.MockValue != nil && !output.MockValue.IsWhollyKnown() {
			return dependencyTarget{}, UnknownMockValueError{ConfigPath: configPath, Output: output.Name}
		}
	}

	return target, nil
}

// unitTarget returns the config of the unit dep reads, false when dep is disabled or reads a stack or a missing unit.
func (dep *Dependency) unitTarget(fsys vfs.FS, pctx *ParsingContext) (string, bool) {
	if dep.isDisabled() {
		return "", false
	}

	configPath, ok := dep.configPathString()
	if !ok {
		return "", false
	}

	targetConfig := getCleanedTargetConfigPath(fsys, configPath, pctx.TerragruntConfigPath)
	if !vfs.Exists(fsys, targetConfig) {
		return "", false
	}

	if stackFile, ok := resolveStackFilePath(configPath, targetConfig); ok && vfs.Exists(fsys, stackFile) {
		return "", false
	}

	return targetConfig, true
}

// UnitsWithoutOutputs returns the configs of the units that the dependencies of cfg read and that declare no outputs.
func UnitsWithoutOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfg *TerragruntConfig,
) ([]string, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil, nil
	}

	targets := make([]string, 0, len(cfg.TerragruntDependencies))
	errs := make([]error, 0, len(cfg.TerragruntDependencies))

	for i := range cfg.TerragruntDependencies {
		target, err := cfg.TerragruntDependencies[i].readTarget(ctx, l, v, pctx)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if target.configPath != "" && target.outputs == nil {
			targets = append(targets, target.configPath)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	slices.Sort(targets)

	return slices.Compact(targets), nil
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

// mockValueMap returns the entries of a known object or map by key, each with the marks of mocks, or false otherwise.
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
