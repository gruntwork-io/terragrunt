package config

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Parse the config of the given include, if one is specified
func parseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), includePath)
	}

	return ParseConfigFile(includePath, terragruntOptions, includedConfig)
}

// handleInclude merges the included config into the current config depending on the merge strategy specified by the
// user.
func handleInclude(
	config *TerragruntConfig,
	terragruntInclude *terragruntInclude,
	terragruntOptions *options.TerragruntOptions,
) (*TerragruntConfig, error) {
	mergeStrategy, err := terragruntInclude.Include.GetMergeStrategy()
	if err != nil {
		return config, err
	}

	switch mergeStrategy {
	case NoMerge:
		terragruntOptions.Logger.Debugf("Included config %s has strategy no merge: not merging config in.", terragruntInclude.Include.Path)
		return config, nil
	case ShallowMerge:
		terragruntOptions.Logger.Debugf("Included config %s has strategy shallow merge: merging config in (shallow).", terragruntInclude.Include.Path)
		includedConfig, err := parseIncludedConfig(terragruntInclude.Include, terragruntOptions)
		if err != nil {
			return nil, err
		}
		includedConfig.Merge(config, terragruntOptions)
		return includedConfig, nil
	}

	return nil, errors.WithStackTrace(fmt.Errorf("Impossible condition"))
}

// Merge performs a shallow merge of the given sourceConfig into the targetConfig. sourceConfig will override common
// attributes defined in the targetConfig. Note that this will modify the targetConfig.
func (targetConfig *TerragruntConfig) Merge(sourceConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) {
	// Merge simple attributes first
	if sourceConfig.DownloadDir != "" {
		targetConfig.DownloadDir = sourceConfig.DownloadDir
	}

	if sourceConfig.IamRole != "" {
		targetConfig.IamRole = sourceConfig.IamRole
	}

	if sourceConfig.IamAssumeRoleDuration != nil {
		targetConfig.IamAssumeRoleDuration = sourceConfig.IamAssumeRoleDuration
	}

	if sourceConfig.TerraformVersionConstraint != "" {
		targetConfig.TerraformVersionConstraint = sourceConfig.TerraformVersionConstraint
	}

	if sourceConfig.TerraformBinary != "" {
		targetConfig.TerraformBinary = sourceConfig.TerraformBinary
	}

	if sourceConfig.PreventDestroy != nil {
		targetConfig.PreventDestroy = sourceConfig.PreventDestroy
	}

	if sourceConfig.RetryMaxAttempts != nil {
		targetConfig.RetryMaxAttempts = sourceConfig.RetryMaxAttempts
	}

	if sourceConfig.RetrySleepIntervalSec != nil {
		targetConfig.RetrySleepIntervalSec = sourceConfig.RetrySleepIntervalSec
	}

	if sourceConfig.TerragruntVersionConstraint != "" {
		targetConfig.TerragruntVersionConstraint = sourceConfig.TerragruntVersionConstraint
	}

	// Skip has to be set specifically in each file that should be skipped
	targetConfig.Skip = sourceConfig.Skip

	if sourceConfig.RemoteState != nil {
		targetConfig.RemoteState = sourceConfig.RemoteState
	}

	if sourceConfig.Terraform != nil {
		if targetConfig.Terraform == nil {
			targetConfig.Terraform = sourceConfig.Terraform
		} else {
			if sourceConfig.Terraform.Source != nil {
				targetConfig.Terraform.Source = sourceConfig.Terraform.Source
			}
			mergeExtraArgs(terragruntOptions, sourceConfig.Terraform.ExtraArgs, &targetConfig.Terraform.ExtraArgs)

			mergeHooks(terragruntOptions, sourceConfig.Terraform.BeforeHooks, &targetConfig.Terraform.BeforeHooks)
			mergeHooks(terragruntOptions, sourceConfig.Terraform.AfterHooks, &targetConfig.Terraform.AfterHooks)
		}
	}

	if sourceConfig.Dependencies != nil {
		targetConfig.Dependencies = sourceConfig.Dependencies
	}

	if sourceConfig.RetryableErrors != nil {
		targetConfig.RetryableErrors = sourceConfig.RetryableErrors
	}

	// Merge the generate configs. This is a shallow merge. Meaning, if the child has the same name generate block, then the
	// child's generate block will override the parent's block.
	for key, val := range sourceConfig.GenerateConfigs {
		targetConfig.GenerateConfigs[key] = val
	}

	if sourceConfig.Inputs != nil {
		targetConfig.Inputs = mergeInputs(sourceConfig.Inputs, targetConfig.Inputs)
	}
}

// Merge the extra arguments.
//
// If a child's extra_arguments has the same name a parent's extra_arguments,
// then the child's extra_arguments will be selected (and the parent's ignored)
// If a child's extra_arguments has a different name from all of the parent's extra_arguments,
// then the child's extra_arguments will be added to the end  of the parents.
// Therefore, terragrunt will put the child extra_arguments after the parent's
// extra_arguments on the terraform cli.
// Therefore, if .tfvar files from both the parent and child contain a variable
// with the same name, the value from the child will win.
func mergeExtraArgs(terragruntOptions *options.TerragruntOptions, childExtraArgs []TerraformExtraArguments, parentExtraArgs *[]TerraformExtraArguments) {
	result := *parentExtraArgs
	for _, child := range childExtraArgs {
		parentExtraArgsWithSameName := getIndexOfExtraArgsWithName(result, child.Name)
		if parentExtraArgsWithSameName != -1 {
			// If the parent contains an extra_arguments with the same name as the child,
			// then override the parent's extra_arguments with the child's.
			terragruntOptions.Logger.Debugf("extra_arguments '%v' from child overriding parent", child.Name)
			result[parentExtraArgsWithSameName] = child
		} else {
			// If the parent does not contain an extra_arguments with the same name as the child
			// then add the child to the end.
			// This ensures the child extra_arguments are added to the command line after the parent extra_arguments.
			result = append(result, child)
		}
	}
	*parentExtraArgs = result
}

func mergeInputs(childInputs map[string]interface{}, parentInputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}

	for key, value := range parentInputs {
		out[key] = value
	}

	for key, value := range childInputs {
		out[key] = value
	}

	return out
}

// Merge the hooks (before_hook and after_hook).
//
// If a child's hook (before_hook or after_hook) has the same name a parent's hook,
// then the child's hook will be selected (and the parent's ignored)
// If a child's hook has a different name from all of the parent's hooks,
// then the child's hook will be added to the end of the parent's.
// Therefore, the child with the same name overrides the parent
func mergeHooks(terragruntOptions *options.TerragruntOptions, childHooks []Hook, parentHooks *[]Hook) {
	result := *parentHooks
	for _, child := range childHooks {
		parentHookWithSameName := getIndexOfHookWithName(result, child.Name)
		if parentHookWithSameName != -1 {
			// If the parent contains a hook with the same name as the child,
			// then override the parent's hook with the child's.
			terragruntOptions.Logger.Debugf("hook '%v' from child overriding parent", child.Name)
			result[parentHookWithSameName] = child
		} else {
			// If the parent does not contain a hook with the same name as the child
			// then add the child to the end.
			result = append(result, child)
		}
	}
	*parentHooks = result
}
