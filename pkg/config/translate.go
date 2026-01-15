package config

import (
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
)

// ToRunConfig translates a TerragruntConfig to a runcfg.RunConfig.
// This is the primary method for converting config types to runner types.
func (cfg *TerragruntConfig) ToRunConfig() *runcfg.RunConfig {
	if cfg == nil {
		return nil
	}

	return &runcfg.RunConfig{
		Terraform:                   translateTerraformConfig(cfg.Terraform),
		RemoteState:                 cfg.RemoteState,
		Exclude:                     translateExcludeConfig(cfg.Exclude),
		GenerateConfigs:             cfg.GenerateConfigs,
		Inputs:                      cfg.Inputs,
		IAMRole:                     cfg.GetIAMRoleOptions(),
		DownloadDir:                 cfg.DownloadDir,
		TerraformBinary:             cfg.TerraformBinary,
		TerraformVersionConstraint:  cfg.TerraformVersionConstraint,
		TerragruntVersionConstraint: cfg.TerragruntVersionConstraint,
		PreventDestroy:              cfg.PreventDestroy,
		ProcessedIncludes:           translateIncludeConfigs(cfg.ProcessedIncludes),
		Dependencies:                translateModuleDependencies(cfg.Dependencies),
		Engine:                      translateEngineConfig(cfg.Engine),
		Errors:                      translateErrorsConfig(cfg.Errors),
	}
}

// translateTerraformConfig converts config.TerraformConfig to runcfg.TerraformConfig.
func translateTerraformConfig(tf *TerraformConfig) *runcfg.TerraformConfig {
	if tf == nil {
		return nil
	}

	return &runcfg.TerraformConfig{
		Source:                tf.Source,
		IncludeInCopy:         tf.IncludeInCopy,
		ExcludeFromCopy:       tf.ExcludeFromCopy,
		CopyTerraformLockFile: tf.CopyTerraformLockFile,
		ExtraArgs:             translateExtraArgs(tf.ExtraArgs),
		BeforeHooks:           translateHooks(tf.BeforeHooks),
		AfterHooks:            translateHooks(tf.AfterHooks),
		ErrorHooks:            translateErrorHooks(tf.ErrorHooks),
	}
}

// translateExtraArgs converts []TerraformExtraArguments to []runcfg.TerraformExtraArguments.
func translateExtraArgs(args []TerraformExtraArguments) []runcfg.TerraformExtraArguments {
	if args == nil {
		return nil
	}

	result := make([]runcfg.TerraformExtraArguments, len(args))
	for i, arg := range args {
		result[i] = runcfg.TerraformExtraArguments{
			Name:             arg.Name,
			Commands:         arg.Commands,
			Arguments:        arg.Arguments,
			RequiredVarFiles: arg.RequiredVarFiles,
			OptionalVarFiles: arg.OptionalVarFiles,
			EnvVars:          arg.EnvVars,
		}
	}

	return result
}

// translateHooks converts []Hook to []runcfg.Hook.
func translateHooks(hooks []Hook) []runcfg.Hook {
	if hooks == nil {
		return nil
	}

	result := make([]runcfg.Hook, len(hooks))
	for i, hook := range hooks {
		result[i] = runcfg.Hook{
			Name:           hook.Name,
			Commands:       hook.Commands,
			Execute:        hook.Execute,
			WorkingDir:     hook.WorkingDir,
			RunOnError:     hook.RunOnError,
			If:             hook.If,
			SuppressStdout: hook.SuppressStdout,
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
		result[i] = runcfg.ErrorHook{
			Name:           hook.Name,
			Commands:       hook.Commands,
			Execute:        hook.Execute,
			OnErrors:       hook.OnErrors,
			WorkingDir:     hook.WorkingDir,
			SuppressStdout: hook.SuppressStdout,
		}
	}

	return result
}

// translateExcludeConfig converts *ExcludeConfig to *runcfg.ExcludeConfig.
func translateExcludeConfig(exclude *ExcludeConfig) *runcfg.ExcludeConfig {
	if exclude == nil {
		return nil
	}

	return &runcfg.ExcludeConfig{
		If:                  exclude.If,
		Actions:             exclude.Actions,
		ExcludeDependencies: exclude.ExcludeDependencies,
		NoRun:               exclude.NoRun,
	}
}

// translateIncludeConfigs converts IncludeConfigsMap to map[string]runcfg.IncludeConfig.
func translateIncludeConfigs(includes IncludeConfigsMap) map[string]runcfg.IncludeConfig {
	if includes == nil {
		return nil
	}

	result := make(map[string]runcfg.IncludeConfig, len(includes))
	for name, inc := range includes {
		result[name] = runcfg.IncludeConfig{
			Name:          inc.Name,
			Path:          inc.Path,
			Expose:        inc.Expose,
			MergeStrategy: inc.MergeStrategy,
		}
	}

	return result
}

// translateModuleDependencies converts *ModuleDependencies to *runcfg.ModuleDependencies.
func translateModuleDependencies(deps *ModuleDependencies) *runcfg.ModuleDependencies {
	if deps == nil {
		return nil
	}

	return &runcfg.ModuleDependencies{
		Paths: deps.Paths,
	}
}

// translateEngineConfig converts *EngineConfig to *runcfg.EngineConfig.
func translateEngineConfig(engine *EngineConfig) *runcfg.EngineConfig {
	if engine == nil {
		return nil
	}

	return &runcfg.EngineConfig{
		Source:  engine.Source,
		Version: engine.Version,
		Type:    engine.Type,
		Meta:    engine.Meta,
	}
}

// translateErrorsConfig converts *ErrorsConfig to *runcfg.ErrorsConfig.
func translateErrorsConfig(errors *ErrorsConfig) *runcfg.ErrorsConfig {
	if errors == nil {
		return nil
	}

	return &runcfg.ErrorsConfig{
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
