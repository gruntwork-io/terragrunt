package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/imdario/mergo"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const bareIncludeKey = ""

// Parse the config of the given include, if one is specified
func parseIncludedConfig(
	includedConfig *IncludeConfig,
	terragruntOptions *options.TerragruntOptions,
	dependencyOutputs *cty.Value,
	decodeList []PartialDecodeSectionType,
) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), includePath)
	}

	// These condition are here to specifically handle the `run-all` command. During any `run-all` call, terragrunt
	// needs to first build up the dependency graph to know what order to process the modules in. We want to limit users
	// from creating a dependency between the dependency path for graph generation, and a module output. This is because
	// the outputs may not be available yet during the graph generation. E.g., consider a completely new deployment and
	// `terragrunt run-all apply` is called. In this case, the outputs are expected to be materialized while terragrunt
	// is running `apply` through the graph, but NOT when the dependency graph is first being formulated.
	//
	// To support this, we implement the following conditions for when terragrunt can fully parse the included config
	// (only one needs to be true):
	// - Included config does NOT have a dependency block.
	// - Terragrunt is NOT performing a partial parse (which indicates whether or not Terragrunt is building a module
	//   graph).
	//
	// These conditions are sufficient to avoid a situation where dependency block parsing relies on output fetching.
	// Note that the user does not have to have a dynamic dependency path that directly depends on dependency outputs to
	// cause this! For example, suppose the user has a dependency path that depends on an included input:
	//
	// include "root" {
	//   path = find_in_parent_folders()
	//   expose = true
	// }
	// dependency "dep" {
	//   config_path = include.root.inputs.vpc_dir
	// }
	//
	// In this example, the user the vpc_dir input may not directly depend on a dependency. However, what if the root
	// config had other inputs that depended on a dependency? E.g.:
	//
	// inputs = {
	//   vpc_dir = "../vpc"
	//   vpc_id  = dependency.vpc.outputs.id
	// }
	//
	// In this situation, terragrunt can not parse the included inputs attribute unless it fetches the `vpc` dependency
	// outputs. Since the block parsing is transitive, it leads to a situation where terragrunt cannot parse the `dep`
	// dependency block unless the `vpc` dependency has outputs (since we can't partially parse the `inputs` attribute).
	// OTOH, if we know the included config has no `dependency` defined, then no matter what attribute is pulled in, we
	// know that the `dependency` block path will never depend on dependency outputs. Hence, we perform a full
	// parse of the included config in the graph generation stage only if the included config does NOT have a dependency
	// block, but resort to a partial parse otherwise.
	//
	// NOTE: To make the logic easier to implement, we implement the inverse here, where we check whether the included
	// config has a dependency block, and if we are in the middle of a partial parse, we perform a partial parse of the
	// included config.
	hasDependency, err := configFileHasDependencyBlock(includePath, terragruntOptions)
	if err != nil {
		return nil, err
	}
	if hasDependency && len(decodeList) > 0 {
		terragruntOptions.Logger.Debugf(
			"Included config %s can only be partially parsed during dependency graph formation for run-all command as it has a dependency block.",
			includePath,
		)
		return PartialParseConfigFile(includePath, terragruntOptions, includedConfig, decodeList)
	}
	return ParseConfigFile(includePath, terragruntOptions, includedConfig, dependencyOutputs)
}

// handleInclude merges the included config into the current config depending on the merge strategy specified by the
// user.
func handleInclude(
	config *TerragruntConfig,
	trackInclude *TrackInclude,
	terragruntOptions *options.TerragruntOptions,
	dependencyOutputs *cty.Value,
) (*TerragruntConfig, error) {
	if trackInclude == nil {
		return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: HANDLE_INCLUDE_NIL_INCLUDE_CONFIG")
	}

	// We merge in the include blocks in reverse order here. The expectation is that the bottom most elements override
	// those in earlier includes, so we need to merge bottom up instead of top down to ensure this.
	includeList := trackInclude.CurrentList
	baseConfig := config
	for i := len(includeList) - 1; i >= 0; i-- {
		includeConfig := includeList[i]
		mergeStrategy, err := includeConfig.GetMergeStrategy()
		if err != nil {
			return config, err
		}

		parsedIncludeConfig, err := parseIncludedConfig(&includeConfig, terragruntOptions, dependencyOutputs, nil)
		if err != nil {
			return nil, err
		}

		switch mergeStrategy {
		case NoMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy no merge: not merging config in.", includeConfig.Path)
		case ShallowMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy shallow merge: merging config in (shallow).", includeConfig.Path)
			parsedIncludeConfig.Merge(baseConfig, terragruntOptions)
			baseConfig = parsedIncludeConfig
		case DeepMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy deep merge: merging config in (deep).", includeConfig.Path)
			if err := parsedIncludeConfig.DeepMerge(baseConfig, terragruntOptions); err != nil {
				return nil, err
			}
			baseConfig = parsedIncludeConfig
		default:
			return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: UNKNOWN_MERGE_STRATEGY_%s", mergeStrategy)
		}
	}
	return baseConfig, nil
}

// handleIncludePartial merges the a partially parsed include config into the child config according to the strategy
// specified by the user.
func handleIncludePartial(
	config *TerragruntConfig,
	trackInclude *TrackInclude,
	terragruntOptions *options.TerragruntOptions,
	decodeList []PartialDecodeSectionType,
) (*TerragruntConfig, error) {
	if trackInclude == nil {
		return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: HANDLE_INCLUDE_PARTIAL_NIL_INCLUDE_CONFIG")
	}

	// We merge in the include blocks in reverse order here. The expectation is that the bottom most elements override
	// those in earlier includes, so we need to merge bottom up instead of top down to ensure this.
	includeList := trackInclude.CurrentList
	baseConfig := config
	for i := len(includeList) - 1; i >= 0; i-- {
		includeConfig := includeList[i]
		mergeStrategy, err := includeConfig.GetMergeStrategy()
		if err != nil {
			return nil, err
		}

		parsedIncludeConfig, err := partialParseIncludedConfig(&includeConfig, terragruntOptions, decodeList)
		if err != nil {
			return nil, err
		}

		switch mergeStrategy {
		case NoMerge:
			terragruntOptions.Logger.Debugf("[Partial] Included config %s has strategy no merge: not merging config in.", includeConfig.Path)
		case ShallowMerge:
			terragruntOptions.Logger.Debugf("[Partial] Included config %s has strategy shallow merge: merging config in (shallow).", includeConfig.Path)
			parsedIncludeConfig.Merge(baseConfig, terragruntOptions)
			baseConfig = parsedIncludeConfig
		case DeepMerge:
			terragruntOptions.Logger.Debugf("[Partial] Included config %s has strategy deep merge: merging config in (deep).", includeConfig.Path)
			if err := parsedIncludeConfig.DeepMerge(baseConfig, terragruntOptions); err != nil {
				return nil, err
			}
			baseConfig = parsedIncludeConfig
		default:
			return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: UNKNOWN_MERGE_STRATEGY_%s_PARTIAL", mergeStrategy)
		}
	}
	return baseConfig, nil
}

// handleIncludeForDependency is a partial merge of the included config to handle dependencies. This only merges the
// dependency block configurations between the included config and the child config. This allows us to merge the two
// dependencies prior to retrieving the outputs, allowing you to have partial configuration that is overridden by a
// child.
func handleIncludeForDependency(
	childDecodedDependency terragruntDependency,
	trackInclude *TrackInclude,
	terragruntOptions *options.TerragruntOptions,
) (*terragruntDependency, error) {
	if trackInclude == nil {
		return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: HANDLE_INCLUDE_DEPENDENCY_NIL_INCLUDE_CONFIG")
	}
	// We merge in the include blocks in reverse order here. The expectation is that the bottom most elements override
	// those in earlier includes, so we need to merge bottom up instead of top down to ensure this.
	includeList := trackInclude.CurrentList
	baseDependencyBlock := childDecodedDependency.Dependencies
	for i := len(includeList) - 1; i >= 0; i-- {
		includeConfig := includeList[i]
		mergeStrategy, err := includeConfig.GetMergeStrategy()
		if err != nil {
			return nil, err
		}

		includedPartialParse, err := partialParseIncludedConfig(&includeConfig, terragruntOptions, []PartialDecodeSectionType{DependencyBlock})
		if err != nil {
			return nil, err
		}

		switch mergeStrategy {
		case NoMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy no merge: not merging config in for dependency.", includeConfig.Path)
		case ShallowMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy shallow merge: merging config in (shallow) for dependency.", includeConfig.Path)
			mergedDependencyBlock := mergeDependencyBlocks(includedPartialParse.TerragruntDependencies, baseDependencyBlock)
			baseDependencyBlock = mergedDependencyBlock
		case DeepMerge:
			terragruntOptions.Logger.Debugf("Included config %s has strategy deep merge: merging config in (deep) for dependency.", includeConfig.Path)
			mergedDependencyBlock, err := deepMergeDependencyBlocks(includedPartialParse.TerragruntDependencies, baseDependencyBlock)
			if err != nil {
				return nil, err
			}
			baseDependencyBlock = mergedDependencyBlock
		default:
			return nil, fmt.Errorf("You reached an impossible condition. This is most likely a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this error message. Code: UNKNOWN_MERGE_STRATEGY_%s_DEPENDENCY", mergeStrategy)
		}
	}
	return &terragruntDependency{Dependencies: baseDependencyBlock}, nil
}

// Merge performs a shallow merge of the given sourceConfig into the targetConfig. sourceConfig will override common
// attributes defined in the targetConfig. Note that this will modify the targetConfig.
// NOTE: the following attributes are deliberately omitted from the merge operation, as they are handled differently in
// the parser:
//     - locals [These blocks are not merged by design]
// NOTE: dependencies block is a special case and is merged deeply. This is necessary to ensure the configstack system
// works correctly, as it uses the `Dependencies` list to track the dependencies of modules for graph building purposes.
// This list includes the dependencies added from dependency blocks, which is handled in a different stage.
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
			mergeErrorHooks(terragruntOptions, sourceConfig.Terraform.ErrorHooks, &targetConfig.Terraform.ErrorHooks)
		}
	}

	// Dependency blocks are shallow merged by name
	targetConfig.TerragruntDependencies = mergeDependencyBlocks(targetConfig.TerragruntDependencies, sourceConfig.TerragruntDependencies)

	// Deep merge the dependencies list. This is different from dependency blocks, and refers to the deprecated
	// dependencies block!
	if sourceConfig.Dependencies != nil {
		if targetConfig.Dependencies == nil {
			targetConfig.Dependencies = sourceConfig.Dependencies
		} else {
			targetConfig.Dependencies.Merge(sourceConfig.Dependencies)
		}
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

	copyFieldsMetadata(sourceConfig, targetConfig)
}

// DeepMerge performs a deep merge of the given sourceConfig into the targetConfig. Deep merge is defined as follows:
// - For simple types, the source overrides the target.
// - For lists, the two attribute lists are combined together in concatenation.
// - For maps, the two maps are combined together recursively. That is, if the map keys overlap, then a deep merge is
//   performed on the map value.
// - Note that some structs are not deep mergeable due to an implementation detail. This will change in the future. The
//   following structs have this limitation:
//     - remote_state
//     - generate
// - Note that the following attributes are deliberately omitted from the merge operation, as they are handled
//   differently in the parser:
//     - dependency blocks (TerragruntDependencies) [These blocks need to retrieve outputs, so we need to merge during
//       the parsing step, not after the full config is decoded]
//     - locals [These blocks are not merged by design]
func (targetConfig *TerragruntConfig) DeepMerge(sourceConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) error {
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

	// Copy only dependencies which doesn't exist in source
	if sourceConfig.Dependencies != nil {
		resultModuleDependencies := &ModuleDependencies{}
		if targetConfig.Dependencies != nil {
			// take in result dependencies only paths which aren't defined in source
			// Fix for issue: https://github.com/gruntwork-io/terragrunt/issues/1900
			targetPathMap := fetchDependencyPaths(targetConfig)
			sourcePathMap := fetchDependencyPaths(sourceConfig)
			for key, value := range targetPathMap {
				_, found := sourcePathMap[key]
				if !found {
					resultModuleDependencies.Paths = append(resultModuleDependencies.Paths, value)
				}
			}
			// copy target paths which are defined only in Dependencies and not in TerragruntDependencies
			// if TerragruntDependencies will be empty, all targetConfig.Dependencies.Paths will be copied to resultModuleDependencies.Paths
			for _, dependencyPath := range targetConfig.Dependencies.Paths {
				var addPath = true
				for _, targetPath := range targetPathMap {
					if dependencyPath == targetPath { // path already defined in TerragruntDependencies, skip adding
						addPath = false
						break
					}
				}
				if addPath {
					resultModuleDependencies.Paths = append(resultModuleDependencies.Paths, dependencyPath)
				}
			}
		}
		resultModuleDependencies.Paths = append(resultModuleDependencies.Paths, sourceConfig.Dependencies.Paths...)
		targetConfig.Dependencies = resultModuleDependencies
	}

	// Dependency blocks are deep merged by name
	mergedDeps, err := deepMergeDependencyBlocks(targetConfig.TerragruntDependencies, sourceConfig.TerragruntDependencies)
	if err != nil {
		return err
	}
	targetConfig.TerragruntDependencies = mergedDeps

	if sourceConfig.RetryableErrors != nil {
		targetConfig.RetryableErrors = append(targetConfig.RetryableErrors, sourceConfig.RetryableErrors...)
	}

	// Handle complex structs by recursively merging the structs together
	if sourceConfig.Terraform != nil {
		if targetConfig.Terraform == nil {
			targetConfig.Terraform = sourceConfig.Terraform
		} else {
			if sourceConfig.Terraform.Source != nil {
				targetConfig.Terraform.Source = sourceConfig.Terraform.Source
			}

			if sourceConfig.Terraform.IncludeInCopy != nil {
				srcList := *sourceConfig.Terraform.IncludeInCopy
				if targetConfig.Terraform.IncludeInCopy != nil {
					targetList := *targetConfig.Terraform.IncludeInCopy
					combinedList := append(srcList, targetList...)
					targetConfig.Terraform.IncludeInCopy = &combinedList
				} else {
					targetConfig.Terraform.IncludeInCopy = &srcList
				}
			}

			mergeExtraArgs(terragruntOptions, sourceConfig.Terraform.ExtraArgs, &targetConfig.Terraform.ExtraArgs)

			mergeHooks(terragruntOptions, sourceConfig.Terraform.BeforeHooks, &targetConfig.Terraform.BeforeHooks)
			mergeHooks(terragruntOptions, sourceConfig.Terraform.AfterHooks, &targetConfig.Terraform.AfterHooks)
			mergeErrorHooks(terragruntOptions, sourceConfig.Terraform.ErrorHooks, &targetConfig.Terraform.ErrorHooks)
		}
	}

	if sourceConfig.Inputs != nil {
		mergedInputs, err := deepMergeInputs(sourceConfig.Inputs, targetConfig.Inputs)
		if err != nil {
			return err
		}
		targetConfig.Inputs = mergedInputs
	}

	// MAINTAINER'S NOTE: The following structs cannot be deep merged due to an implementation detail (they do not
	// support nil attributes, so we can't determine if an attribute was intentionally set, or was defaulted from
	// unspecified - this is especially problematic for bool attributes).
	if sourceConfig.RemoteState != nil {
		targetConfig.RemoteState = sourceConfig.RemoteState
	}
	for key, val := range sourceConfig.GenerateConfigs {
		targetConfig.GenerateConfigs[key] = val
	}

	copyFieldsMetadata(sourceConfig, targetConfig)
	return nil
}

// fetchDependencyMap - return from configuration map with dependency_name: path
func fetchDependencyPaths(config *TerragruntConfig) map[string]string {
	var m = make(map[string]string)
	if config == nil {
		return m
	}
	for _, dependency := range config.TerragruntDependencies {
		m[dependency.Name] = dependency.ConfigPath
	}
	return m
}

// Merge dependency blocks shallowly. If the source list has the same name as the target, it will override the
// dependency block in the target. Otherwise, the blocks are appended.
func mergeDependencyBlocks(targetDependencies []Dependency, sourceDependencies []Dependency) []Dependency {
	// We track the keys so that the dependencies are added in order, with those in target prepending those in
	// source. This is not strictly necessary, but it makes testing easier by making the output list more
	// predictable.
	keys := []string{}

	dependencyBlocks := make(map[string]Dependency)
	for _, dep := range targetDependencies {
		dependencyBlocks[dep.Name] = dep
		keys = append(keys, dep.Name)
	}
	for _, dep := range sourceDependencies {
		_, hasSameKey := dependencyBlocks[dep.Name]
		if !hasSameKey {
			keys = append(keys, dep.Name)
		}
		// Regardless of what is in dependencyBlocks, we will always override the key with source
		dependencyBlocks[dep.Name] = dep
	}
	// Now convert the map to list and set target
	combinedDeps := []Dependency{}
	for _, key := range keys {
		combinedDeps = append(combinedDeps, dependencyBlocks[key])
	}
	return combinedDeps
}

// Merge dependency blocks deeply. This works almost the same as mergeDependencyBlocks, except it will recursively merge
// attributes of the dependency struct if they share the same name.
func deepMergeDependencyBlocks(targetDependencies []Dependency, sourceDependencies []Dependency) ([]Dependency, error) {
	// We track the keys so that the dependencies are added in order, with those in target prepending those in
	// source. This is not strictly necessary, but it makes testing easier by making the output list more
	// predictable.
	keys := []string{}

	dependencyBlocks := make(map[string]Dependency)
	for _, dep := range targetDependencies {
		dependencyBlocks[dep.Name] = dep
		keys = append(keys, dep.Name)
	}
	for _, dep := range sourceDependencies {
		sameKeyDep, hasSameKey := dependencyBlocks[dep.Name]
		if hasSameKey {
			sameKeyDepPtr := &sameKeyDep
			if err := sameKeyDepPtr.DeepMerge(dep); err != nil {
				return nil, err
			}
			dependencyBlocks[dep.Name] = *sameKeyDepPtr
		} else {
			dependencyBlocks[dep.Name] = dep
			keys = append(keys, dep.Name)
		}

	}
	// Now convert the map to list and set target
	combinedDeps := []Dependency{}
	for _, key := range keys {
		combinedDeps = append(combinedDeps, dependencyBlocks[key])
	}
	return combinedDeps, nil
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

func deepMergeInputs(childInputs map[string]interface{}, parentInputs map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for key, value := range parentInputs {
		out[key] = value
	}

	err := mergo.Merge(&out, childInputs, mergo.WithAppendSlice, mergo.WithOverride)
	return out, errors.WithStackTrace(err)
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

// Merge the error hooks (error_hook).
// Does the same thing as mergeHooks but for error hooks
// TODO: Figure out more DRY way to do this
func mergeErrorHooks(terragruntOptions *options.TerragruntOptions, childHooks []ErrorHook, parentHooks *[]ErrorHook) {
	result := *parentHooks
	for _, child := range childHooks {
		parentHookWithSameName := getIndexOfErrorHookWithName(result, child.Name)
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

// getTrackInclude converts the terragrunt include blocks into TrackInclude structs that differentiate between an
// included config in the current parsing context, and an included config that was passed through from a previous
// parsing context.
func getTrackInclude(
	terragruntIncludeList []IncludeConfig,
	includeFromChild *IncludeConfig,
	terragruntOptions *options.TerragruntOptions,
) (*TrackInclude, error) {
	includedPaths := []string{}
	terragruntIncludeMap := make(map[string]IncludeConfig, len(terragruntIncludeList))
	for _, tgInc := range terragruntIncludeList {
		includedPaths = append(includedPaths, tgInc.Path)
		terragruntIncludeMap[tgInc.Name] = tgInc
	}

	hasInclude := len(terragruntIncludeList) > 0
	if hasInclude && includeFromChild != nil {
		// tgInc appears in a parent that is already included, which means a nested include block. This is not
		// something we currently support.
		err := errors.WithStackTrace(TooManyLevelsOfInheritance{
			ConfigPath:             terragruntOptions.TerragruntConfigPath,
			FirstLevelIncludePath:  includeFromChild.Path,
			SecondLevelIncludePath: strings.Join(includedPaths, ","),
		})
		return nil, err
	} else if hasInclude && includeFromChild == nil {
		// Current parsing context where there is no included config already loaded.
		trackInc := TrackInclude{
			CurrentList: terragruntIncludeList,
			CurrentMap:  terragruntIncludeMap,
			Original:    nil,
		}
		return &trackInc, nil
	} else {
		// Parsing context where there is an included config already loaded.
		trackInc := TrackInclude{
			CurrentList: terragruntIncludeList,
			CurrentMap:  terragruntIncludeMap,
			Original:    includeFromChild,
		}
		return &trackInc, nil
	}
}

// updateBareIncludeBlock searches the parsed terragrunt contents for a bare include block (include without a label),
// and convert it to one with empty string as the label. This is necessary because the hcl parser is strictly enforces
// label counts when parsing out labels with a go struct.
//
// Returns the updated contents, a boolean indicated whether anything changed, and an error (if any).
func updateBareIncludeBlock(file *hcl.File, filename string) ([]byte, bool, error) {
	if filepath.Ext(filename) == ".json" {
		return updateBareIncludeBlockJSON(file.Bytes)
	}

	hclFile, diags := hclwrite.ParseConfig(file.Bytes, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, false, errors.WithStackTrace(diags)
	}

	codeWasUpdated := false
	for _, block := range hclFile.Body().Blocks() {
		if block.Type() == "include" && len(block.Labels()) == 0 {
			if codeWasUpdated {
				return nil, false, errors.WithStackTrace(MultipleBareIncludeBlocksErr{})
			}
			block.SetLabels([]string{bareIncludeKey})
			codeWasUpdated = true
		}
	}
	return hclFile.Bytes(), codeWasUpdated, nil
}

// updateBareIncludeBlockJSON implements the logic for updateBareIncludeBlock when the terragrunt.hcl configuration is
// encoded in json. The json version of this function is fairly complex due to the flexibility in how the blocks are
// encoded. That is, all of the following are valid encodings of a terragrunt.hcl.json file that has a bare include
// block:
//
// Case 1: a single include block as top level:
// {
//   "include": {
//     "path": "foo"
//   }
// }
//
// Case 2: a single include block in list:
// {
//   "include": [
//     {"path": "foo"}
//   ]
// }
//
// Case 3: mixed bare and labeled include block as list:
// {
//   "include": [
//     {"path": "foo"},
//     {
//       "labeled": {"path": "bar"}
//     }
//   ]
// }
//
// For simplicity of implementation, we focus on handling Case 1 and 2, and ignore Case 3. If we see Case 3, we will
// error out. Instead, the user should handle this case explicitly using the object encoding instead of list encoding:
// {
//   "include": {
//     "": {"path": "foo"},
//     "labeled": {"path": "bar"}
//   }
// }
// If the multiple include blocks are encoded in this way in the json configuration, nothing needs to be done by this
// function.
func updateBareIncludeBlockJSON(fileBytes []byte) ([]byte, bool, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(fileBytes, &parsed); err != nil {
		return nil, false, errors.WithStackTrace(err)
	}
	includeBlock, hasKey := parsed["include"]
	if !hasKey {
		// No include block, so don't do anything
		return fileBytes, false, nil
	}
	switch typed := includeBlock.(type) {
	case []interface{}:
		if len(typed) == 0 {
			// No include block, so don't do anything
			return nil, false, nil
		} else if len(typed) > 1 {
			// Could be multiple bare includes, or Case 3. We simplify the handling of this case by erroring out,
			// ignoring the possibility of Case 3, which, while valid HCL encoding, is too complex to detect and handle
			// here. Instead we will recommend users use the object encoding.
			return nil, false, errors.WithStackTrace(MultipleBareIncludeBlocksErr{})
		}

		// Make sure this is Case 2, and not Case 3 with a single labeled block. If Case 2, update to inject the labeled
		// version. Otherwise, return without modifying.
		singleBlock := typed[0]
		if jsonIsIncludeBlock(singleBlock) {
			return updateSingleBareIncludeInParsedJSON(parsed, singleBlock)
		}
		return nil, false, nil
	case map[string]interface{}:
		if len(typed) == 0 {
			// No include block, so don't do anything
			return nil, false, nil
		}

		// We will only update the include block if we detect the object to represent an include block. Otherwise, the
		// blocks are labeled so we can pass forward to the tg parser step.
		if jsonIsIncludeBlock(typed) {
			return updateSingleBareIncludeInParsedJSON(parsed, typed)
		}
		return nil, false, nil
	}

	return nil, false, errors.WithStackTrace(IncludeIsNotABlockErr{parsed: includeBlock})
}

// updateBareIncludeInParsedJSON replaces the include attribute into a block with the label "" in the json. Note that we
// can directly assign to the map with the single "" key without worrying about the possibility of other include blocks
// since we will only call this function if there is only one include block, and that is a bare block with no labels.
func updateSingleBareIncludeInParsedJSON(parsed map[string]interface{}, newVal interface{}) ([]byte, bool, error) {
	parsed["include"] = map[string]interface{}{bareIncludeKey: newVal}
	updatedBytes, err := json.Marshal(parsed)
	return updatedBytes, true, errors.WithStackTrace(err)
}

// jsonIsIncludeBlock checks if the arbitrary json data is the include block. The data is determined to be an include
// block if:
// - It is an object
// - Has the 'path' attribute
// - The 'path' attribute is a string
func jsonIsIncludeBlock(jsonData interface{}) bool {
	typed, isMap := jsonData.(map[string]interface{})
	if isMap {
		pathAttr, hasPath := typed["path"]
		if hasPath {
			_, pathIsString := pathAttr.(string)
			return pathIsString
		}
	}
	return false
}

// copyFieldsMetadata Copy fields metadata between TerragruntConfig instances.
func copyFieldsMetadata(sourceConfig *TerragruntConfig, targetConfig *TerragruntConfig) {
	if sourceConfig.FieldsMetadata != nil {
		if targetConfig.FieldsMetadata == nil {
			targetConfig.FieldsMetadata = map[string]map[string]interface{}{}
		}
		for k, v := range sourceConfig.FieldsMetadata {
			targetConfig.FieldsMetadata[k] = v
		}
	}
}

// Custom error types

type MultipleBareIncludeBlocksErr struct{}

func (err MultipleBareIncludeBlocksErr) Error() string {
	return "Multiple bare include blocks (include blocks without label) is not supported."
}

type IncludeIsNotABlockErr struct {
	parsed interface{}
}

func (err IncludeIsNotABlockErr) Error() string {
	return fmt.Sprintf("Parsed include is not a block: %v", err.parsed)
}
