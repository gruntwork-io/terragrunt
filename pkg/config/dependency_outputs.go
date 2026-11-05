package config

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// withDeclaredOutputs adds the mock values and mock_strategy the unit at config_path declares to a copy of dep,
// with dep's own settings winning, and returns that copy with the declared contract. It returns dep itself and no
// contract when the experiment is off, the dependency is disabled, config_path names no unit config, or the unit
// declares no outputs block, which gives the outputs-block tip.
func (dep *Dependency) withDeclaredOutputs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (*Dependency, *OutputsConfig, error) {
	if !pctx.Experiments.Evaluate(experiment.OutputsBlock) || dep.isDisabled() {
		return dep, nil, nil
	}

	configPath, ok := dep.configPathString()
	if !ok {
		return dep, nil, nil
	}

	targetConfig := getCleanedTargetConfigPath(v.FS, configPath, pctx.TerragruntConfigPath)
	if !vfs.Exists(v.FS, targetConfig) {
		return dep, nil, nil
	}

	declared, err := ReadDeclaredOutputs(ctx, l, v, pctx, targetConfig)
	if err != nil {
		return nil, nil, err
	}

	if declared == nil {
		if pctx.OutputsBlockTip != nil {
			pctx.OutputsBlockTip(l, targetConfig)
		}

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

// ReadDeclaredOutputs returns the outputs block of the unit config at targetConfig, merged with those of the
// configs it includes, or nil when it declares none. The read decodes that block alone and leaves the unit's own
// dependency outputs unresolved, so reading a contract never fetches the outputs of the unit's dependencies.
//
// Returns [DeclaredOutputsReadError] when the config cannot be read.
func ReadDeclaredOutputs(
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

	cfg, err := PartialParseConfigFile(ctx, l, v, targetPctx, targetConfig, nil)
	if err != nil {
		return nil, DeclaredOutputsReadError{TargetConfig: targetConfig, Err: err}
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

	if dep.shouldMergeMockOutputsWithState(pctx) && dep.MockOutputs != nil &&
		!dep.MockOutputs.IsNull() && dep.MockOutputs.IsKnown() && dep.MockOutputs.CanIterateElements() {
		mocks = dep.MockOutputs.AsValueMap()
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
