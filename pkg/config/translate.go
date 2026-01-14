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
		FeatureFlags:                translateFeatureFlags(cfg.FeatureFlags),
		ProcessedIncludes:           translateIncludeConfigs(cfg.ProcessedIncludes),
		Dependencies:                translateModuleDependencies(cfg.Dependencies),
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

// translateFeatureFlags converts FeatureFlags to []runcfg.FeatureFlag.
func translateFeatureFlags(flags FeatureFlags) []runcfg.FeatureFlag {
	if flags == nil {
		return nil
	}

	result := make([]runcfg.FeatureFlag, 0, len(flags))
	for _, flag := range flags {
		var defaultVal any
		if flag.Default != nil {
			// Convert cty.Value to any - this is a simplified conversion
			// In practice, feature flags are typically simple types
			defaultVal = flag.Default.GoString()
		}

		result = append(result, runcfg.FeatureFlag{
			Name:    flag.Name,
			Default: defaultVal,
		})
	}

	return result
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
