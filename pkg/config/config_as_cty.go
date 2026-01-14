package config

import (
	"encoding/json"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
)

// TerragruntConfigAsCty serializes TerragruntConfig struct to a cty Value that can be used to reference the attributes in other config. Note
// that we can't straight up convert the struct using cty tags due to differences in the desired representation.
// Specifically, we want to reference blocks by named attributes, but blocks are rendered to lists in the
// TerragruntConfig struct, so we need to do some massaging of the data to convert the list of blocks in to a map going
// from the block name label to the block value.
func TerragruntConfigAsCty(config *TerragruntConfig) (cty.Value, error) {
	output := map[string]cty.Value{}

	// Convert attributes that are primitive types
	output[MetadataTerraformBinary] = gostringToCty(config.TerraformBinary)
	output[MetadataTerraformVersionConstraint] = gostringToCty(config.TerraformVersionConstraint)
	output[MetadataTerragruntVersionConstraint] = gostringToCty(config.TerragruntVersionConstraint)
	output[MetadataDownloadDir] = gostringToCty(config.DownloadDir)
	output[MetadataIamRole] = gostringToCty(config.IamRole)
	output[MetadataIamAssumeRoleSessionName] = gostringToCty(config.IamAssumeRoleSessionName)
	output[MetadataIamWebIdentityToken] = gostringToCty(config.IamWebIdentityToken)

	catalogConfigCty, err := catalogConfigAsCty(config.Catalog)
	if err != nil {
		return cty.NilVal, err
	}

	if catalogConfigCty != cty.NilVal {
		output[MetadataCatalog] = catalogConfigCty
	}

	engineConfigCty, err := engineConfigAsCty(config.Engine)
	if err != nil {
		return cty.NilVal, err
	}

	if engineConfigCty != cty.NilVal {
		output[MetadataEngine] = engineConfigCty
	}

	excludeConfigCty, err := excludeConfigAsCty(config.Exclude)
	if err != nil {
		return cty.NilVal, err
	}

	if excludeConfigCty != cty.NilVal {
		output[MetadataExclude] = excludeConfigCty
	}

	errorsConfigCty, err := errorsConfigAsCty(config.Errors)
	if err != nil {
		return cty.NilVal, err
	}

	if errorsConfigCty != cty.NilVal {
		output[MetadataErrors] = errorsConfigCty
	}

	terraformConfigCty, err := terraformConfigAsCty(config.Terraform)
	if err != nil {
		return cty.NilVal, err
	}

	if terraformConfigCty != cty.NilVal {
		output[MetadataTerraform] = terraformConfigCty
	}

	remoteStateCty, err := RemoteStateAsCty(config.RemoteState)
	if err != nil {
		return cty.NilVal, err
	}

	if remoteStateCty != cty.NilVal {
		output[MetadataRemoteState] = remoteStateCty
	}

	dependenciesCty, err := GoTypeToCty(config.Dependencies)
	if err != nil {
		return cty.NilVal, err
	}

	if dependenciesCty != cty.NilVal {
		output[MetadataDependencies] = dependenciesCty
	}

	if config.PreventDestroy != nil {
		output[MetadataPreventDestroy] = goboolToCty(*config.PreventDestroy)
	}

	dependencyCty, err := dependencyBlocksAsCty(config.TerragruntDependencies)
	if err != nil {
		return cty.NilVal, err
	}

	if dependencyCty != cty.NilVal {
		output[MetadataDependency] = dependencyCty
	}

	generateCty, err := GoTypeToCty(config.GenerateConfigs)
	if err != nil {
		return cty.NilVal, err
	}

	if generateCty != cty.NilVal {
		output[MetadataGenerateConfigs] = generateCty
	}

	iamAssumeRoleDurationCty, err := GoTypeToCty(config.IamAssumeRoleDuration)
	if err != nil {
		return cty.NilVal, err
	}

	if iamAssumeRoleDurationCty != cty.NilVal {
		output[MetadataIamAssumeRoleDuration] = iamAssumeRoleDurationCty
	}

	inputsCty, err := convertToCtyWithJSON(config.Inputs)
	if err != nil {
		return cty.NilVal, err
	}

	if inputsCty != cty.NilVal {
		output[MetadataInputs] = inputsCty
	}

	localsCty, err := convertToCtyWithJSON(config.Locals)
	if err != nil {
		return cty.NilVal, err
	}

	if localsCty != cty.NilVal {
		output[MetadataLocals] = localsCty
	}

	if config.DependentModulesPath != nil {
		dependentModulesCty, err := convertToCtyWithJSON(config.DependentModulesPath)
		if err != nil {
			return cty.NilVal, err
		}

		if dependentModulesCty != cty.NilVal {
			output[MetadataDependentModules] = dependentModulesCty
		}
	}

	featureFlagsCty, err := featureFlagsBlocksAsCty(config.FeatureFlags)
	if err != nil {
		return cty.NilVal, err
	}

	if featureFlagsCty != cty.NilVal {
		output[MetadataFeatureFlag] = featureFlagsCty
	}

	return ConvertValuesMapToCtyVal(output)
}

func TerragruntConfigAsCtyWithMetadata(config *TerragruntConfig) (cty.Value, error) {
	output := map[string]cty.Value{}

	// Convert attributes that are primitive types
	if err := wrapWithMetadata(config, config.TerraformBinary, MetadataTerraformBinary, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.TerraformVersionConstraint, MetadataTerraformVersionConstraint, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.TerragruntVersionConstraint, MetadataTerragruntVersionConstraint, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.DownloadDir, MetadataDownloadDir, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.IamRole, MetadataIamRole, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.IamAssumeRoleSessionName, MetadataIamAssumeRoleSessionName, &output); err != nil {
		return cty.NilVal, err
	}

	if config.PreventDestroy != nil {
		if err := wrapWithMetadata(config, *config.PreventDestroy, MetadataPreventDestroy, &output); err != nil {
			return cty.NilVal, err
		}
	}

	if err := wrapWithMetadata(config, config.IamAssumeRoleDuration, MetadataIamAssumeRoleDuration, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.DependentModulesPath, MetadataDependentModules, &output); err != nil {
		return cty.NilVal, err
	}

	// Terraform
	terraformConfigCty, err := terraformConfigAsCty(config.Terraform)
	if err != nil {
		return cty.NilVal, err
	}

	if terraformConfigCty != cty.NilVal {
		if err := wrapWithMetadata(config, terraformConfigCty, MetadataTerraform, &output); err != nil {
			return cty.NilVal, err
		}
	}

	// Remote state
	remoteStateCty, err := RemoteStateAsCty(config.RemoteState)
	if err != nil {
		return cty.NilVal, err
	}

	if remoteStateCty != cty.NilVal {
		if err := wrapWithMetadata(config, remoteStateCty, MetadataRemoteState, &output); err != nil {
			return cty.NilVal, err
		}
	}

	if err := wrapCtyMapWithMetadata(config, &config.Inputs, MetadataInputs, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapCtyMapWithMetadata(config, &config.Locals, MetadataLocals, &output); err != nil {
		return cty.NilVal, err
	}

	// remder dependencies as list of maps with "value" and "metadata"
	if config.Dependencies != nil {
		var dependencyWithMetadata = make([]ValueWithMetadata, 0, len(config.Dependencies.Paths))

		for _, dependency := range config.Dependencies.Paths {
			var content = ValueWithMetadata{}

			content.Value = gostringToCty(dependency)

			metadata, found := config.GetMapFieldMetadata(MetadataDependencies, dependency)
			if found {
				content.Metadata = metadata
			}

			dependencyWithMetadata = append(dependencyWithMetadata, content)
		}

		dependenciesCty, err := GoTypeToCty(dependencyWithMetadata)
		if err != nil {
			return cty.NilVal, err
		}

		output[MetadataDependencies] = dependenciesCty
	}

	if config.TerragruntDependencies != nil {
		var dependenciesMap = map[string]cty.Value{}

		for _, block := range config.TerragruntDependencies {
			ctyValue, err := GoTypeToCty(block)
			if err != nil {
				continue
			}

			if ctyValue == cty.NilVal {
				continue
			}

			var content = ValueWithMetadata{}

			content.Value = ctyValue

			metadata, found := config.GetMapFieldMetadata(MetadataDependency, block.Name)
			if found {
				content.Metadata = metadata
			}

			value, err := GoTypeToCty(content)
			if err != nil {
				continue
			}

			dependenciesMap[block.Name] = value
		}

		if len(dependenciesMap) > 0 {
			dependenciesCty, err := ConvertValuesMapToCtyVal(dependenciesMap)
			if err != nil {
				return cty.NilVal, err
			}

			output[MetadataDependency] = dependenciesCty
		}
	}

	if config.GenerateConfigs != nil {
		var generateConfigsWithMetadata = map[string]cty.Value{}

		for key, value := range config.GenerateConfigs {
			ctyValue, err := GoTypeToCty(value)
			if err != nil {
				continue
			}

			if ctyValue == cty.NilVal {
				continue
			}

			var content = ValueWithMetadata{}

			content.Value = ctyValue

			metadata, found := config.GetMapFieldMetadata(MetadataGenerateConfigs, key)
			if found {
				content.Metadata = metadata
			}

			v, err := GoTypeToCty(content)
			if err != nil {
				continue
			}

			generateConfigsWithMetadata[key] = v
		}

		if len(generateConfigsWithMetadata) > 0 {
			dependenciesCty, err := ConvertValuesMapToCtyVal(generateConfigsWithMetadata)
			if err != nil {
				return cty.NilVal, err
			}

			output[MetadataGenerateConfigs] = dependenciesCty
		}
	}

	return ConvertValuesMapToCtyVal(output)
}

func wrapCtyMapWithMetadata(config *TerragruntConfig, data *map[string]any, fieldType string, output *map[string]cty.Value) error {
	var valueWithMetadata = map[string]cty.Value{}

	for key, value := range *data {
		var content = ValueWithMetadata{}

		ctyValue, err := convertToCtyWithJSON(value)
		if err != nil {
			return err
		}

		content.Value = ctyValue

		metadata, found := config.GetMapFieldMetadata(fieldType, key)
		if found {
			content.Metadata = metadata
		}

		v, err := GoTypeToCty(content)
		if err != nil {
			continue
		}

		valueWithMetadata[key] = v
	}

	if len(valueWithMetadata) > 0 {
		localsCty, err := ConvertValuesMapToCtyVal(valueWithMetadata)
		if err != nil {
			return err
		}

		(*output)[fieldType] = localsCty
	}

	return nil
}

func wrapWithMetadata(config *TerragruntConfig, value any, metadataName string, output *map[string]cty.Value) error {
	if value == nil {
		return nil
	}

	var valueWithMetadata = ValueWithMetadata{}

	ctyValue, err := GoTypeToCty(value)
	if err != nil {
		return err
	}

	valueWithMetadata.Value = ctyValue

	metadata, found := config.GetFieldMetadata(metadataName)
	if found {
		valueWithMetadata.Metadata = metadata
	}

	ctyJSON, err := GoTypeToCty(valueWithMetadata)
	if err != nil {
		return err
	}

	if ctyJSON != cty.NilVal {
		(*output)[metadataName] = ctyJSON
	}

	return nil
}

// ValueWithMetadata stores value and metadata used in render-json with metadata.
type ValueWithMetadata struct {
	Value    cty.Value         `json:"value" cty:"value"`
	Metadata map[string]string `json:"metadata" cty:"metadata"`
}

// ctyCatalogConfig is an alternate representation of CatalogConfig that converts internal blocks into a map that
// maps the name to the underlying struct, as opposed to a list representation.
type ctyCatalogConfig struct {
	URLs []string `cty:"urls"`
}

// ctyEngineConfig is an alternate representation of EngineConfig that converts internal blocks into a map that
// maps the name to the underlying struct, as opposed to a list representation.
type ctyEngineConfig struct {
	Meta    cty.Value `cty:"meta"`
	Source  string    `cty:"source"`
	Version string    `cty:"version"`
	Type    string    `cty:"type"`
}

// ctyExclude exclude representation for cty.
type ctyExclude struct {
	Actions             []string `cty:"actions"`
	If                  bool     `cty:"if"`
	ExcludeDependencies bool     `cty:"exclude_dependencies"`
}

// Serialize CatalogConfig to a cty Value, but with maps instead of lists for the blocks.
func catalogConfigAsCty(config *CatalogConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	configCty := ctyCatalogConfig{
		URLs: config.URLs,
	}

	return GoTypeToCty(configCty)
}

// Serialize engineConfigAsCty to a cty Value, but with maps instead of lists for the blocks.
func engineConfigAsCty(config *EngineConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	var v, t string
	if config.Version != nil {
		v = *config.Version
	}

	if config.Type != nil {
		t = *config.Type
	}

	configCty := ctyEngineConfig{
		Source:  config.Source,
		Version: v,
		Type:    t,
	}

	if config.Meta != nil {
		configCty.Meta = *config.Meta
	}

	return GoTypeToCty(configCty)
}

// excludeConfigAsCty serialize exclude configuration to a cty Value.
func excludeConfigAsCty(config *ExcludeConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	excludeDependencies := false
	if config.ExcludeDependencies != nil {
		excludeDependencies = *config.ExcludeDependencies
	}

	configCty := ctyExclude{
		If:                  config.If,
		Actions:             config.Actions,
		ExcludeDependencies: excludeDependencies,
	}

	return GoTypeToCty(configCty)
}

// CtyTerraformConfig is an alternate representation of TerraformConfig that converts internal blocks into a map that
// maps the name to the underlying struct, as opposed to a list representation.
type CtyTerraformConfig struct {
	ExtraArgs             map[string]TerraformExtraArguments `cty:"extra_arguments"`
	Source                *string                            `cty:"source"`
	IncludeInCopy         *[]string                          `cty:"include_in_copy"`
	ExcludeFromCopy       *[]string                          `cty:"exclude_from_copy"`
	CopyTerraformLockFile *bool                              `cty:"copy_terraform_lock_file"`
	BeforeHooks           map[string]Hook                    `cty:"before_hook"`
	AfterHooks            map[string]Hook                    `cty:"after_hook"`
	ErrorHooks            map[string]ErrorHook               `cty:"error_hook"`
}

// Serialize TerraformConfig to a cty Value, but with maps instead of lists for the blocks.
func terraformConfigAsCty(config *TerraformConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	configCty := CtyTerraformConfig{
		Source:                config.Source,
		IncludeInCopy:         config.IncludeInCopy,
		ExcludeFromCopy:       config.ExcludeFromCopy,
		CopyTerraformLockFile: config.CopyTerraformLockFile,
		ExtraArgs:             map[string]TerraformExtraArguments{},
		BeforeHooks:           map[string]Hook{},
		AfterHooks:            map[string]Hook{},
		ErrorHooks:            map[string]ErrorHook{},
	}

	for _, arg := range config.ExtraArgs {
		configCty.ExtraArgs[arg.Name] = arg
	}

	for _, hook := range config.BeforeHooks {
		configCty.BeforeHooks[hook.Name] = hook
	}

	for _, hook := range config.AfterHooks {
		configCty.AfterHooks[hook.Name] = hook
	}

	for _, errorHook := range config.ErrorHooks {
		configCty.ErrorHooks[errorHook.Name] = errorHook
	}

	return GoTypeToCty(configCty)
}

// RemoteStateAsCty serializes RemoteState to a cty Value. We can't directly
// serialize the struct because `config` and `encryption` are arbitrary
// interfaces whose type we do not know, so we have to do a hack to go through json.
func RemoteStateAsCty(remote *remotestate.RemoteState) (cty.Value, error) {
	if remote == nil || remote.Config == nil {
		return cty.NilVal, nil
	}

	config := remote.Config

	output := map[string]cty.Value{}
	output["backend"] = gostringToCty(config.BackendName)
	output["disable_init"] = goboolToCty(config.DisableInit)
	output["disable_dependency_optimization"] = goboolToCty(config.DisableDependencyOptimization)

	generateCty, err := GoTypeToCty(config.Generate)
	if err != nil {
		return cty.NilVal, err
	}

	output["generate"] = generateCty

	ctyJSONVal, err := convertToCtyWithJSON(config.BackendConfig)
	if err != nil {
		return cty.NilVal, err
	}

	output["config"] = ctyJSONVal

	ctyJSONVal, err = convertToCtyWithJSON(config.Encryption)
	if err != nil {
		return cty.NilVal, err
	}

	output["encryption"] = ctyJSONVal

	return ConvertValuesMapToCtyVal(output)
}

// Serialize the list of dependency blocks to a cty Value as a map that maps the block names to the cty representation.
func dependencyBlocksAsCty(dependencyBlocks Dependencies) (cty.Value, error) {
	out := map[string]cty.Value{}

	for _, block := range dependencyBlocks {
		blockCty, err := GoTypeToCty(block)
		if err != nil {
			return cty.NilVal, err
		}

		out[block.Name] = blockCty
	}

	return ConvertValuesMapToCtyVal(out)
}

// Serialize the list of feature flags to a cty Value as a map that maps the feature names to the cty representation.
func featureFlagsBlocksAsCty(featureFlagBlocks FeatureFlags) (cty.Value, error) {
	out := map[string]cty.Value{}

	for _, feature := range featureFlagBlocks {
		featureCty, err := GoTypeToCty(feature)
		if err != nil {
			return cty.NilVal, err
		}

		out[feature.Name] = featureCty
	}

	return ConvertValuesMapToCtyVal(out)
}

// Serialize errors configuration as cty.Value.
func errorsConfigAsCty(config *ErrorsConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	output := map[string]cty.Value{}

	retryCty, err := GoTypeToCty(config.Retry)
	if err != nil {
		return cty.NilVal, err
	}

	if retryCty != cty.NilVal {
		output[MetadataRetry] = retryCty
	}

	ignoreCty, err := GoTypeToCty(config.Ignore)
	if err != nil {
		return cty.NilVal, err
	}

	if ignoreCty != cty.NilVal {
		output[MetadataIgnore] = ignoreCty
	}

	return ConvertValuesMapToCtyVal(output)
}

// stackConfigAsCty converts a StackConfig into a cty Value so its attributes can be used in other configs.
func stackConfigAsCty(stackConfig *StackConfig) (cty.Value, error) {
	if stackConfig == nil {
		return cty.NilVal, nil
	}

	output := map[string]cty.Value{}

	if stackConfig.Locals != nil {
		localsCty, err := convertToCtyWithJSON(stackConfig.Locals)
		if err != nil {
			return cty.NilVal, err
		}

		if localsCty != cty.NilVal {
			output[MetadataLocal] = localsCty
		}
	}

	// Process stacks as a map from stack name to stack config
	if len(stackConfig.Stacks) > 0 {
		stacksMap := make(map[string]cty.Value, len(stackConfig.Stacks))

		for _, stack := range stackConfig.Stacks {
			stackCty, err := stackToCty(stack)
			if err != nil {
				return cty.NilVal, err
			}

			if stackCty != cty.NilVal {
				stacksMap[stack.Name] = stackCty
			}
		}

		if len(stacksMap) > 0 {
			stacksCty, err := ConvertValuesMapToCtyVal(stacksMap)
			if err != nil {
				return cty.NilVal, err
			}

			output[MetadataStack] = stacksCty
		}
	}

	// Process units as a map from unit name to unit config
	if len(stackConfig.Units) > 0 {
		unitsMap := make(map[string]cty.Value, len(stackConfig.Units))

		for _, unit := range stackConfig.Units {
			unitCty, err := unitToCty(unit)
			if err != nil {
				return cty.NilVal, err
			}

			if unitCty != cty.NilVal {
				unitsMap[unit.Name] = unitCty
			}
		}

		if len(unitsMap) > 0 {
			unitsCty, err := ConvertValuesMapToCtyVal(unitsMap)
			if err != nil {
				return cty.NilVal, err
			}

			output[MetadataUnit] = unitsCty
		}
	}

	return ConvertValuesMapToCtyVal(output)
}

// stackToCty converts a Stack struct to a cty Value
func stackToCty(stack *Stack) (cty.Value, error) {
	if stack == nil {
		return cty.NilVal, nil
	}

	output := map[string]cty.Value{
		"name":   gostringToCty(stack.Name),
		"source": gostringToCty(stack.Source),
		"path":   gostringToCty(stack.Path),
	}

	// Handle Values if available
	if stack.Values != nil {
		output["values"] = *stack.Values
	}

	// Handle NoStack if available
	if stack.NoStack != nil {
		output["no_dot_terragrunt_stack"] = goboolToCty(*stack.NoStack)
	}

	if stack.NoValidation != nil {
		output["no_validation"] = goboolToCty(*stack.NoValidation)
	}

	return ConvertValuesMapToCtyVal(output)
}

// unitToCty converts a Unit struct to a cty Value
func unitToCty(unit *Unit) (cty.Value, error) {
	if unit == nil {
		return cty.NilVal, nil
	}

	output := map[string]cty.Value{
		"name":   gostringToCty(unit.Name),
		"source": gostringToCty(unit.Source),
		"path":   gostringToCty(unit.Path),
	}

	// Handle Values if available
	if unit.Values != nil {
		output["values"] = *unit.Values
	}

	// Handle NoStack if available
	if unit.NoStack != nil {
		output["no_dot_terragrunt_stack"] = goboolToCty(*unit.NoStack)
	}

	if unit.NoValidation != nil {
		output["no_validation"] = goboolToCty(*unit.NoValidation)
	}

	return ConvertValuesMapToCtyVal(output)
}

// Converts arbitrary go types that are json serializable to a cty Value by using json as an intermediary
// representation. This avoids the strict type nature of cty, where you need to know the output type beforehand to
// serialize to cty.
func convertToCtyWithJSON(val any) (cty.Value, error) {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return cty.NilVal, errors.New(err)
	}

	var ctyJSONVal ctyjson.SimpleJSONValue
	if err := ctyJSONVal.UnmarshalJSON(jsonBytes); err != nil {
		return cty.NilVal, errors.New(err)
	}

	return ctyJSONVal.Value, nil
}

// GoTypeToCty converts arbitrary go type (struct that has cty tags, slice, map with string keys, string, bool, int
// uint, float, cty.Value) to a cty Value
func GoTypeToCty(val any) (cty.Value, error) {
	// Check if the value is a map
	if m, ok := val.(map[string]any); ok {
		convertedMap := make(map[string]cty.Value)

		for k, v := range m {
			convertedValue, err := GoTypeToCty(v)
			if err != nil {
				return cty.NilVal, err
			}

			convertedMap[k] = convertedValue
		}

		return cty.ObjectVal(convertedMap), nil
	}

	// Use the existing logic for other types
	ctyType, err := gocty.ImpliedType(val)
	if err != nil {
		return cty.NilVal, errors.New(err)
	}

	ctyOut, err := gocty.ToCtyValue(val, ctyType)
	if err != nil {
		return cty.NilVal, errors.New(err)
	}

	return ctyOut, nil
}

// Converts primitive go strings to a cty Value.
func gostringToCty(val string) cty.Value {
	ctyOut, err := gocty.ToCtyValue(val, cty.String)
	if err != nil {
		// Since we are converting primitive strings, we should never get an error in this conversion.
		panic(err)
	}

	return ctyOut
}

// Converts primitive go bools to a cty Value.
func goboolToCty(val bool) cty.Value {
	ctyOut, err := gocty.ToCtyValue(val, cty.Bool)
	if err != nil {
		// Since we are converting primitive bools, we should never get an error in this conversion.
		panic(err)
	}

	return ctyOut
}

// FormatValue converts a primitive value to its string representation.
func FormatValue(value cty.Value) (string, error) {
	if value.Type() == cty.String {
		return value.AsString(), nil
	}

	return GetValueString(value)
}
