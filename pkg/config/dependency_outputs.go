package config

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type declaredDependency struct {
	dep     *Dependency
	outputs *declaredOutputs
}

// declaredOutputs is the outputs block a dependency reads, with the mock values that fill state for one command.
type declaredOutputs struct {
	config *OutputsConfig
	merges []stateMerge
}

type stateMerge struct {
	mocks    cty.Value
	strategy MergeStrategyType
}

// withDeclaredOutputs returns dep with the mock values and mock strategy that its unit declares filled in.
func (dep *Dependency) withDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (declaredDependency, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return declaredDependency{dep: dep}, nil
	}

	declared, err := dep.DeclaredOutputs(ctx, l, v, s, pctx)
	if err != nil {
		return declaredDependency{}, err
	}

	return dep.withDeclared(pctx, declared), nil
}

// withDeclared returns dep with the mock values and mock strategy that declared gives the command of pctx.
func (dep *Dependency) withDeclared(pctx *ParsingContext, declared *OutputsConfig) declaredDependency {
	if declared == nil {
		return declaredDependency{dep: dep}
	}

	effective := *dep
	effective.MockOutputs = mergeDeclaredMockValues(declared.MockValues(), dep.MockOutputs)

	dependentCommands := dep.MockOutputsAllowedTerraformCommands != nil
	dependentMerge := dep.MockOutputsMergeStrategyWithState != nil || dep.MockOutputsMergeWithState != nil

	if strategy := declared.MockStrategy; strategy != nil {
		if !dependentCommands && strategy.AllowedCommands != nil {
			effective.MockOutputsAllowedTerraformCommands = strategy.AllowedCommands
		}

		if !dependentMerge && strategy.MergeWithState != nil {
			effective.MockOutputsMergeStrategyWithState = strategy.MergeWithState
		}
	}

	read := &declaredOutputs{config: declared}

	values, keyed := mockValueMap(effective.MockOutputs)
	if !keyed || len(values) == 0 {
		read.merges = effective.stateMerges(pctx)

		return declaredDependency{dep: &effective, outputs: read}
	}

	fallback := make(map[string]cty.Value, len(values))
	merges := map[MergeStrategyType]map[string]cty.Value{}
	render := isRenderJSONCommand(pctx) || isRenderCommand(pctx)

	for key, value := range values {
		commands := effective.MockOutputsAllowedTerraformCommands
		merge := effective.getMockOutputsMergeStrategy()
		own := declared.outputStrategy(key)

		if !dependentCommands && own.AllowedCommands != nil {
			commands = own.AllowedCommands
		}

		if !dependentMerge && own.MergeWithState != nil {
			merge = *own.MergeWithState
		}

		allowed := mockCommandAllowed(pctx, commands)
		if allowed || render {
			fallback[key] = value
		}

		if !allowed || merge == NoMerge {
			continue
		}

		if merges[merge] == nil {
			merges[merge] = map[string]cty.Value{}
		}

		merges[merge][key] = value
	}

	for _, strategy := range slices.Sorted(maps.Keys(merges)) {
		read.merges = append(read.merges, stateMerge{strategy: strategy, mocks: cty.ObjectVal(merges[strategy])})
	}

	effective.MockOutputsAllowedTerraformCommands = nil
	effective.MockOutputs = keptMockValues(effective.MockOutputs, fallback, len(values))

	return declaredDependency{dep: &effective, outputs: read}
}

// stackUnitRead returns how dependencyConfig reads the stack unit at unitConfigPath, as it is without an outputs block.
func stackUnitRead(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	dependencyConfig *Dependency,
	unitConfigPath string,
	unitAddress []string,
) (declaredDependency, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return declaredDependency{dep: dependencyConfig}, nil
	}

	unitDep := *dependencyConfig
	unitDep.ConfigPath = cty.StringVal(unitConfigPath)
	unitDep.MockOutputs = nil

	declared, err := unitDep.DeclaredOutputs(ctx, l, v, s, pctx)
	if err != nil {
		return declaredDependency{}, err
	}

	if declared == nil {
		return declaredDependency{dep: dependencyConfig}, nil
	}

	mock, ok, err := stackMockOutput(dependencyConfig, unitAddress)
	if err != nil {
		return declaredDependency{}, err
	}

	if ok {
		unitDep.MockOutputs = &mock
	}

	return unitDep.withDeclared(pctx, declared), nil
}

// stackUnitMock returns the mock value of the stack unit at address, false when the command reads none.
func (d declaredDependency) stackUnitMock(
	pctx *ParsingContext,
	dependencyConfig *Dependency,
	address []string,
) (cty.Value, bool, error) {
	if d.outputs == nil {
		return stackMockOutput(dependencyConfig, address)
	}

	if d.dep.MockOutputs == nil || !d.dep.shouldReturnMockOutputs(pctx) {
		return cty.NilVal, false, nil
	}

	return *d.dep.MockOutputs, true, nil
}

// DeclaredOutputs returns the outputs block of the unit dep reads, nil when there is none, or [DependencyBlockError].
func (dep *Dependency) DeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (*OutputsConfig, error) {
	target, err := dep.readTarget(ctx, l, v, s, pctx)
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
	s *Store,
	pctx *ParsingContext,
) (dependencyTarget, error) {
	configPath, ok := dep.unitTarget(v.FS, pctx)
	if !ok {
		return dependencyTarget{}, nil
	}

	single := TerragruntDependency{Dependencies: Dependencies{*dep}}
	if _, err := decodeDependencies(ctx, l, v, s, pctx, single); err != nil {
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
	s *Store,
	pctx *ParsingContext,
	cfg *TerragruntConfig,
) ([]string, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil, nil
	}

	targets := make([]string, 0, len(cfg.TerragruntDependencies))
	errs := make([]error, 0, len(cfg.TerragruntDependencies))

	for i := range cfg.TerragruntDependencies {
		target, err := cfg.TerragruntDependencies[i].readTarget(ctx, l, v, s, pctx)
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

// checkDeclaredOutputs converts each state output to the type it declares, or reports the outputs state lacks.
func checkDeclaredOutputs(
	pctx *ParsingContext,
	declared *declaredOutputs,
	targetConfig string,
	stateOutputs map[string]cty.Value,
) error {
	if declared == nil || isRenderJSONCommand(pctx) || isRenderCommand(pctx) {
		return nil
	}

	filled := declared.filledOutputs()
	errs := make([]error, 0, len(declared.config.Outputs))

	for _, output := range declared.config.Outputs {
		value, inState := stateOutputs[output.Name]
		if !inState {
			if _, ok := filled[output.Name]; !ok {
				errs = append(errs, DeclaredOutputMissingError{TargetConfig: targetConfig, Output: output.Name})
			}

			continue
		}

		conformed, err := output.Conform(value)
		if err != nil {
			errs = append(errs, DeclaredOutputTypeError{
				TargetConfig: targetConfig,
				Output:       output.Name,
				Type:         typeexpr.TypeString(output.Type),
				Err:          err,
			})

			continue
		}

		stateOutputs[output.Name] = conformed
	}

	return errors.Join(errs...)
}

// mergedWithState returns state with the mock values of d that fill it for the command.
func (d *declaredOutputs) mergedWithState(state cty.Value) (*cty.Value, error) {
	merged := state

	for _, merge := range d.merges {
		filled, err := mergeMockOutputsWithState(merge.strategy, merged, merge.mocks)
		if err != nil {
			return nil, err
		}

		merged = withDeclaredMockMarks(merge.strategy, *filled, merge.mocks, merged)
	}

	return &merged, nil
}

// filledOutputs returns the names of the outputs whose mock values stand in for outputs the state lacks.
func (d *declaredOutputs) filledOutputs() map[string]struct{} {
	filled := map[string]struct{}{}

	for i := range d.merges {
		values, ok := mockValueMap(&d.merges[i].mocks)
		if !ok {
			continue
		}

		for name := range values {
			filled[name] = struct{}{}
		}
	}

	return filled
}

// stateMerges returns the merge of the mock values of dep with state, none when the command merges nothing.
func (dep *Dependency) stateMerges(pctx *ParsingContext) []stateMerge {
	if dep.MockOutputs == nil || !dep.shouldMergeMockOutputsWithState(pctx) {
		return nil
	}

	return []stateMerge{{strategy: dep.getMockOutputsMergeStrategy(), mocks: *dep.MockOutputs}}
}

// outputStrategy returns the mock_strategy of the output named name, or an empty one when it sets none.
func (c *OutputsConfig) outputStrategy(name string) OutputsMockStrategy {
	declaration := c.Declaration(name)
	if declaration == nil || declaration.MockStrategy == nil {
		return OutputsMockStrategy{}
	}

	return *declaration.MockStrategy
}

// mockCommandAllowed reports whether commands lets the command of pctx read mock values.
func mockCommandAllowed(pctx *ParsingContext, commands *[]string) bool {
	return commands == nil || len(*commands) == 0 || slices.Contains(*commands, pctx.OriginalTofuCommand)
}

// keptMockValues returns mocks when kept holds each of its total values, the kept ones otherwise, nil for none.
func keptMockValues(mocks *cty.Value, kept map[string]cty.Value, total int) *cty.Value {
	if len(kept) == total {
		return mocks
	}

	if len(kept) == 0 {
		return nil
	}

	return new(cty.ObjectVal(kept))
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
