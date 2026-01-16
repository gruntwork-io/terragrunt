package config

import (
	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ToRunConfig translates a TerragruntConfig to a runcfg.RunConfig.
// This is the primary method for converting config types to runner types.
func (cfg *TerragruntConfig) ToRunConfig(l log.Logger) *runcfg.RunConfig {
	if cfg == nil {
		return nil
	}

	runCfg := &runcfg.RunConfig{
		Terraform:                   translateTerraformConfig(cfg.Terraform, l),
		RemoteState:                 translateRemoteState(cfg.RemoteState),
		Exclude:                     translateExcludeConfig(cfg.Exclude),
		GenerateConfigs:             translateGenerateConfigs(cfg.GenerateConfigs),
		Inputs:                      translateInputs(cfg.Inputs),
		IAMRole:                     cfg.GetIAMRoleOptions(),
		DownloadDir:                 cfg.DownloadDir,
		TerraformBinary:             cfg.TerraformBinary,
		TerraformVersionConstraint:  cfg.TerraformVersionConstraint,
		TerragruntVersionConstraint: cfg.TerragruntVersionConstraint,
		PreventDestroy:              translatePreventDestroy(cfg.PreventDestroy),
		ProcessedIncludes:           translateProcessedIncludes(cfg.ProcessedIncludes),
		Dependencies:                translateModuleDependencies(cfg.Dependencies),
		Engine:                      translateEngineConfig(cfg.Engine),
		Errors:                      translateErrorsConfig(cfg.Errors),
	}

	return runCfg
}

// translateTerraformConfig converts config.TerraformConfig to runcfg.TerraformConfig.
func translateTerraformConfig(tf *TerraformConfig, l log.Logger) runcfg.TerraformConfig {
	if tf == nil {
		return runcfg.TerraformConfig{}
	}

	var source string
	if tf.Source != nil {
		source = *tf.Source
	}

	var includeInCopy []string
	if tf.IncludeInCopy != nil {
		includeInCopy = *tf.IncludeInCopy
	}

	var excludeFromCopy []string
	if tf.ExcludeFromCopy != nil {
		excludeFromCopy = *tf.ExcludeFromCopy
	}

	// Default to true (copy) when not set, as per original behavior
	copyTerraformLockFile := true
	if tf.CopyTerraformLockFile != nil {
		copyTerraformLockFile = *tf.CopyTerraformLockFile
	}

	return runcfg.TerraformConfig{
		Source:                source,
		IncludeInCopy:         includeInCopy,
		ExcludeFromCopy:       excludeFromCopy,
		CopyTerraformLockFile: copyTerraformLockFile,
		ExtraArgs:             translateExtraArgs(tf.ExtraArgs, l),
		BeforeHooks:           translateHooks(tf.BeforeHooks),
		AfterHooks:            translateHooks(tf.AfterHooks),
		ErrorHooks:            translateErrorHooks(tf.ErrorHooks),
	}
}

// translateExtraArgs converts []TerraformExtraArguments to []runcfg.TerraformExtraArguments.
func translateExtraArgs(args []TerraformExtraArguments, l log.Logger) []runcfg.TerraformExtraArguments {
	if args == nil {
		return nil
	}

	result := make([]runcfg.TerraformExtraArguments, len(args))
	for i, arg := range args {
		varFiles := computeVarFiles(arg.RequiredVarFiles, arg.OptionalVarFiles, l)

		var arguments []string
		if arg.Arguments != nil {
			arguments = *arg.Arguments
		}

		var requiredVarFiles []string
		if arg.RequiredVarFiles != nil {
			requiredVarFiles = *arg.RequiredVarFiles
		}

		var optionalVarFiles []string
		if arg.OptionalVarFiles != nil {
			optionalVarFiles = *arg.OptionalVarFiles
		}

		var envVars map[string]string
		if arg.EnvVars != nil {
			envVars = *arg.EnvVars
		}

		result[i] = runcfg.TerraformExtraArguments{
			Name:             arg.Name,
			Commands:         arg.Commands,
			Arguments:        arguments,
			RequiredVarFiles: requiredVarFiles,
			OptionalVarFiles: optionalVarFiles,
			VarFiles:         varFiles,
			EnvVars:          envVars,
		}
	}

	return result
}

// computeVarFiles returns a list of variable files, including required and optional files.
func computeVarFiles(requiredVarFiles *[]string, optionalVarFiles *[]string, l log.Logger) []string {
	var varFiles []string

	// Include all specified RequiredVarFiles.
	if requiredVarFiles != nil {
		varFiles = append(varFiles, util.RemoveDuplicatesKeepLast(*requiredVarFiles)...)
	}

	// If OptionalVarFiles is specified, check for each file if it exists and if so, include in the var
	// files list. Note that it is possible that many files resolve to the same path, so we remove
	// duplicates.
	if optionalVarFiles != nil {
		for _, file := range util.RemoveDuplicatesKeepLast(*optionalVarFiles) {
			if util.FileExists(file) {
				varFiles = append(varFiles, file)
			} else {
				l.Debugf("Skipping var-file %s as it does not exist", file)
			}
		}
	}

	return varFiles
}

// translateHooks converts []Hook to []runcfg.Hook.
func translateHooks(hooks []Hook) []runcfg.Hook {
	if hooks == nil {
		return nil
	}

	result := make([]runcfg.Hook, len(hooks))
	for i, hook := range hooks {
		var workingDir string
		if hook.WorkingDir != nil {
			workingDir = *hook.WorkingDir
		}

		var runOnError bool
		if hook.RunOnError != nil {
			runOnError = *hook.RunOnError
		}

		var ifCondition bool
		if hook.If != nil {
			ifCondition = *hook.If
		}

		var suppressStdout bool
		if hook.SuppressStdout != nil {
			suppressStdout = *hook.SuppressStdout
		}

		result[i] = runcfg.Hook{
			Name:           hook.Name,
			Commands:       hook.Commands,
			Execute:        hook.Execute,
			WorkingDir:     workingDir,
			RunOnError:     runOnError,
			If:             ifCondition,
			SuppressStdout: suppressStdout,
		}
	}

	return result
}

// translateErrorHooks converts []ErrorHook to []runcfg.ErrorHook.
func translateErrorHooks(hooks []ErrorHook) []runcfg.ErrorHook {
	if hooks == nil {
		return nil
	}

	result := make([]runcfg.ErrorHook, len(hooks))
	for i, hook := range hooks {
		var workingDir string
		if hook.WorkingDir != nil {
			workingDir = *hook.WorkingDir
		}

		var suppressStdout bool
		if hook.SuppressStdout != nil {
			suppressStdout = *hook.SuppressStdout
		}

		result[i] = runcfg.ErrorHook{
			Name:           hook.Name,
			Commands:       hook.Commands,
			Execute:        hook.Execute,
			OnErrors:       hook.OnErrors,
			WorkingDir:     workingDir,
			SuppressStdout: suppressStdout,
		}
	}

	return result
}

// translateGenerateConfigs converts map[string]codegen.GenerateConfig to map[string]codegen.GenerateConfig.
// Returns an empty map if the input is nil.
func translateGenerateConfigs(generateConfigs map[string]codegen.GenerateConfig) map[string]codegen.GenerateConfig {
	if generateConfigs == nil {
		return make(map[string]codegen.GenerateConfig)
	}

	return generateConfigs
}

// translateInputs converts map[string]any to map[string]any.
// Returns an empty map if the input is nil.
func translateInputs(inputs map[string]any) map[string]any {
	if inputs == nil {
		return make(map[string]any)
	}

	return inputs
}

// translatePreventDestroy converts *bool to bool.
func translatePreventDestroy(preventDestroy *bool) bool {
	if preventDestroy == nil {
		return false
	}

	return *preventDestroy
}

// translateRemoteState converts *remotestate.RemoteState to remotestate.RemoteState.
func translateRemoteState(remoteState *remotestate.RemoteState) remotestate.RemoteState {
	if remoteState == nil {
		return remotestate.RemoteState{}
	}

	return *remoteState
}

// translateExcludeConfig converts *ExcludeConfig to runcfg.ExcludeConfig.
func translateExcludeConfig(exclude *ExcludeConfig) runcfg.ExcludeConfig {
	if exclude == nil {
		return runcfg.ExcludeConfig{}
	}

	var excludeDependencies bool
	if exclude.ExcludeDependencies != nil {
		excludeDependencies = *exclude.ExcludeDependencies
	}

	var noRun bool
	if exclude.NoRun != nil {
		noRun = *exclude.NoRun
	}

	return runcfg.ExcludeConfig{
		If:                  exclude.If,
		Actions:             exclude.Actions,
		ExcludeDependencies: excludeDependencies,
		NoRun:               noRun,
	}
}

// translateIncludeConfigs converts IncludeConfigsMap to map[string]runcfg.IncludeConfig.
func translateIncludeConfigs(includes IncludeConfigsMap) map[string]runcfg.IncludeConfig {
	if includes == nil {
		return nil
	}

	result := make(map[string]runcfg.IncludeConfig, len(includes))
	for name, inc := range includes {
		var expose bool
		if inc.Expose != nil {
			expose = *inc.Expose
		}

		var mergeStrategy string
		if inc.MergeStrategy != nil {
			mergeStrategy = *inc.MergeStrategy
		}

		result[name] = runcfg.IncludeConfig{
			Name:          inc.Name,
			Path:          inc.Path,
			Expose:        expose,
			MergeStrategy: mergeStrategy,
		}
	}

	return result
}

// translateProcessedIncludes converts IncludeConfigsMap to map[string]runcfg.IncludeConfig.
// Returns an empty map if the input is nil.
func translateProcessedIncludes(includes IncludeConfigsMap) map[string]runcfg.IncludeConfig {
	result := translateIncludeConfigs(includes)
	if result == nil {
		return make(map[string]runcfg.IncludeConfig)
	}

	return result
}

// translateModuleDependencies converts *ModuleDependencies to runcfg.ModuleDependencies.
func translateModuleDependencies(deps *ModuleDependencies) runcfg.ModuleDependencies {
	if deps == nil {
		return runcfg.ModuleDependencies{}
	}

	return runcfg.ModuleDependencies{
		Paths: deps.Paths,
	}
}

// translateEngineConfig converts *EngineConfig to runcfg.EngineConfig.
func translateEngineConfig(engine *EngineConfig) runcfg.EngineConfig {
	if engine == nil {
		return runcfg.EngineConfig{}
	}

	var version string
	if engine.Version != nil {
		version = *engine.Version
	}

	var engineType string
	if engine.Type != nil {
		engineType = *engine.Type
	}

	return runcfg.EngineConfig{
		Source:  engine.Source,
		Version: version,
		Type:    engineType,
		Meta:    engine.Meta,
	}
}

// translateErrorsConfig converts *ErrorsConfig to runcfg.ErrorsConfig.
func translateErrorsConfig(errors *ErrorsConfig) runcfg.ErrorsConfig {
	if errors == nil {
		return runcfg.ErrorsConfig{}
	}

	return runcfg.ErrorsConfig{
		Retry:  translateRetryBlocks(errors.Retry),
		Ignore: translateIgnoreBlocks(errors.Ignore),
	}
}

// translateRetryBlocks converts []*RetryBlock to []*runcfg.RetryBlock.
func translateRetryBlocks(blocks []*RetryBlock) []*runcfg.RetryBlock {
	if blocks == nil {
		return nil
	}

	result := make([]*runcfg.RetryBlock, len(blocks))
	for i, block := range blocks {
		if block == nil {
			continue
		}

		result[i] = &runcfg.RetryBlock{
			Label:            block.Label,
			RetryableErrors:  block.RetryableErrors,
			MaxAttempts:      block.MaxAttempts,
			SleepIntervalSec: block.SleepIntervalSec,
		}
	}

	return result
}

// translateIgnoreBlocks converts []*IgnoreBlock to []*runcfg.IgnoreBlock.
func translateIgnoreBlocks(blocks []*IgnoreBlock) []*runcfg.IgnoreBlock {
	if blocks == nil {
		return nil
	}

	result := make([]*runcfg.IgnoreBlock, len(blocks))
	for i, block := range blocks {
		if block == nil {
			continue
		}

		result[i] = &runcfg.IgnoreBlock{
			Label:           block.Label,
			IgnorableErrors: block.IgnorableErrors,
			Message:         block.Message,
			Signals:         block.Signals,
		}
	}

	return result
}
