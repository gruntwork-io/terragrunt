package config

import (
	"encoding/json"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/remote"
)

// Serialize TerragruntConfig struct to a cty Value that can be used to reference the attributes in other config. Note
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
	output[MetadataSkip] = goboolToCty(config.Skip)
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

	dependenciesCty, err := goTypeToCty(config.Dependencies)
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

	generateCty, err := goTypeToCty(config.GenerateConfigs)
	if err != nil {
		return cty.NilVal, err
	}
	if generateCty != cty.NilVal {
		output[MetadataGenerateConfigs] = generateCty
	}

	retryableCty, err := goTypeToCty(config.RetryableErrors)
	if err != nil {
		return cty.NilVal, err
	}
	if retryableCty != cty.NilVal {
		output[MetadataRetryableErrors] = retryableCty
	}

	iamAssumeRoleDurationCty, err := goTypeToCty(config.IamAssumeRoleDuration)
	if err != nil {
		return cty.NilVal, err
	}

	if iamAssumeRoleDurationCty != cty.NilVal {
		output[MetadataIamAssumeRoleDuration] = iamAssumeRoleDurationCty
	}

	retryMaxAttemptsCty, err := goTypeToCty(config.RetryMaxAttempts)
	if err != nil {
		return cty.NilVal, err
	}
	if retryMaxAttemptsCty != cty.NilVal {
		output[MetadataRetryMaxAttempts] = retryMaxAttemptsCty
	}

	retrySleepIntervalSecCty, err := goTypeToCty(config.RetrySleepIntervalSec)
	if err != nil {
		return cty.NilVal, err
	}
	if retrySleepIntervalSecCty != cty.NilVal {
		output[MetadataRetrySleepIntervalSec] = retrySleepIntervalSecCty
	}

	inputsCty, err := convertToCtyWithJson(config.Inputs)
	if err != nil {
		return cty.NilVal, err
	}
	if inputsCty != cty.NilVal {
		output[MetadataInputs] = inputsCty
	}

	localsCty, err := convertToCtyWithJson(config.Locals)
	if err != nil {
		return cty.NilVal, err
	}
	if localsCty != cty.NilVal {
		output[MetadataLocals] = localsCty
	}

	if len(config.DependentModulesPath) > 0 {
		dependentModulesCty, err := convertToCtyWithJson(config.DependentModulesPath)
		if err != nil {
			return cty.NilVal, err
		}
		if dependentModulesCty != cty.NilVal {
			output[MetadataDependentModules] = dependentModulesCty
		}
	}

	return convertValuesMapToCtyVal(output)
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

	if err := wrapWithMetadata(config, config.Skip, MetadataSkip, &output); err != nil {
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

	if err := wrapWithMetadata(config, config.RetryableErrors, MetadataRetryableErrors, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.IamAssumeRoleDuration, MetadataIamAssumeRoleDuration, &output); err != nil {
		return cty.NilVal, err
	}

	if err := wrapWithMetadata(config, config.RetryMaxAttempts, MetadataRetryMaxAttempts, &output); err != nil {
		return cty.NilVal, err
	}
	if err := wrapWithMetadata(config, config.RetrySleepIntervalSec, MetadataRetrySleepIntervalSec, &output); err != nil {
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
		dependenciesCty, err := goTypeToCty(dependencyWithMetadata)
		if err != nil {
			return cty.NilVal, err
		}
		output[MetadataDependencies] = dependenciesCty
	}

	if config.TerragruntDependencies != nil {
		var dependenciesMap = map[string]cty.Value{}
		for _, block := range config.TerragruntDependencies {
			ctyValue, err := goTypeToCty(block)
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

			value, err := goTypeToCty(content)
			if err != nil {
				continue
			}
			dependenciesMap[block.Name] = value
		}
		if len(dependenciesMap) > 0 {
			dependenciesCty, err := convertValuesMapToCtyVal(dependenciesMap)
			if err != nil {
				return cty.NilVal, err
			}
			output[MetadataDependency] = dependenciesCty
		}
	}

	if config.GenerateConfigs != nil {
		var generateConfigsWithMetadata = map[string]cty.Value{}
		for key, value := range config.GenerateConfigs {
			ctyValue, err := goTypeToCty(value)
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

			v, err := goTypeToCty(content)
			if err != nil {
				continue
			}
			generateConfigsWithMetadata[key] = v
		}
		if len(generateConfigsWithMetadata) > 0 {
			dependenciesCty, err := convertValuesMapToCtyVal(generateConfigsWithMetadata)
			if err != nil {
				return cty.NilVal, err
			}
			output[MetadataGenerateConfigs] = dependenciesCty
		}
	}

	return convertValuesMapToCtyVal(output)
}

func wrapCtyMapWithMetadata(config *TerragruntConfig, data *map[string]interface{}, fieldType string, output *map[string]cty.Value) error {
	var valueWithMetadata = map[string]cty.Value{}
	for key, value := range *data {
		var content = ValueWithMetadata{}
		ctyValue, err := convertToCtyWithJson(value)
		if err != nil {
			return err
		}
		content.Value = ctyValue
		metadata, found := config.GetMapFieldMetadata(fieldType, key)
		if found {
			content.Metadata = metadata
		}
		v, err := goTypeToCty(content)
		if err != nil {
			continue
		}
		valueWithMetadata[key] = v
	}
	if len(valueWithMetadata) > 0 {
		localsCty, err := convertValuesMapToCtyVal(valueWithMetadata)
		if err != nil {
			return err
		}
		(*output)[fieldType] = localsCty
	}
	return nil
}

func wrapWithMetadata(config *TerragruntConfig, value interface{}, metadataName string, output *map[string]cty.Value) error {
	if value == nil {
		return nil
	}
	var valueWithMetadata = ValueWithMetadata{}
	ctyValue, err := goTypeToCty(value)
	if err != nil {
		return err
	}
	valueWithMetadata.Value = ctyValue
	metadata, found := config.GetFieldMetadata(metadataName)
	if found {
		valueWithMetadata.Metadata = metadata
	}
	ctyJson, err := goTypeToCty(valueWithMetadata)
	if err != nil {
		return err
	}
	if ctyJson != cty.NilVal {
		(*output)[metadataName] = ctyJson
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
	Source  string    `cty:"source"`
	Version string    `cty:"version"`
	Type    string    `cty:"type"`
	Meta    cty.Value `cty:"meta"`
}

// Serialize CatalogConfig to a cty Value, but with maps instead of lists for the blocks.
func catalogConfigAsCty(config *CatalogConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	configCty := ctyCatalogConfig{
		URLs: config.URLs,
	}

	return goTypeToCty(configCty)
}

// Serialize engineConfigAsCty to a cty Value, but with maps instead of lists for the blocks.
func engineConfigAsCty(config *EngineConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	ctyMetaVal, err := convertToCtyWithJson(config.Meta)
	if err != nil {
		return cty.NilVal, err
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
		Meta:    ctyMetaVal,
	}

	return goTypeToCty(configCty)
}

// CtyTerraformConfig is an alternate representation of TerraformConfig that converts internal blocks into a map that
// maps the name to the underlying struct, as opposed to a list representation.
type CtyTerraformConfig struct {
	ExtraArgs     map[string]TerraformExtraArguments `cty:"extra_arguments"`
	Source        *string                            `cty:"source"`
	IncludeInCopy *[]string                          `cty:"include_in_copy"`
	BeforeHooks   map[string]Hook                    `cty:"before_hook"`
	AfterHooks    map[string]Hook                    `cty:"after_hook"`
	ErrorHooks    map[string]ErrorHook               `cty:"error_hook"`
}

// Serialize TerraformConfig to a cty Value, but with maps instead of lists for the blocks.
func terraformConfigAsCty(config *TerraformConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	configCty := CtyTerraformConfig{
		Source:        config.Source,
		IncludeInCopy: config.IncludeInCopy,
		ExtraArgs:     map[string]TerraformExtraArguments{},
		BeforeHooks:   map[string]Hook{},
		AfterHooks:    map[string]Hook{},
		ErrorHooks:    map[string]ErrorHook{},
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

	return goTypeToCty(configCty)
}

// Serialize RemoteState to a cty Value. We can't directly serialize the struct because `config` is an arbitrary
// interface whose type we do not know, so we have to do a hack to go through json.
func RemoteStateAsCty(remoteState *remote.RemoteState) (cty.Value, error) {
	if remoteState == nil {
		return cty.NilVal, nil
	}

	output := map[string]cty.Value{}
	output["backend"] = gostringToCty(remoteState.Backend)
	output["disable_init"] = goboolToCty(remoteState.DisableInit)
	output["disable_dependency_optimization"] = goboolToCty(remoteState.DisableDependencyOptimization)

	generateCty, err := goTypeToCty(remoteState.Generate)
	if err != nil {
		return cty.NilVal, err
	}
	output["generate"] = generateCty

	ctyJsonVal, err := convertToCtyWithJson(remoteState.Config)
	if err != nil {
		return cty.NilVal, err
	}
	output["config"] = ctyJsonVal

	return convertValuesMapToCtyVal(output)
}

// Serialize the list of dependency blocks to a cty Value as a map that maps the block names to the cty representation.
func dependencyBlocksAsCty(dependencyBlocks Dependencies) (cty.Value, error) {
	out := map[string]cty.Value{}
	for _, block := range dependencyBlocks {
		blockCty, err := goTypeToCty(block)
		if err != nil {
			return cty.NilVal, err
		}
		out[block.Name] = blockCty
	}
	return convertValuesMapToCtyVal(out)
}

// Converts arbitrary go types that are json serializable to a cty Value by using json as an intermediary
// representation. This avoids the strict type nature of cty, where you need to know the output type beforehand to
// serialize to cty.
func convertToCtyWithJson(val interface{}) (cty.Value, error) {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return cty.NilVal, errors.WithStackTrace(err)
	}
	var ctyJsonVal ctyjson.SimpleJSONValue
	if err := ctyJsonVal.UnmarshalJSON(jsonBytes); err != nil {
		return cty.NilVal, errors.WithStackTrace(err)
	}
	return ctyJsonVal.Value, nil
}

// Converts arbitrary go type (struct that has cty tags, slice, map with string keys, string, bool, int
// uint, float, cty.Value) to a cty Value
func goTypeToCty(val interface{}) (cty.Value, error) {
	ctyType, err := gocty.ImpliedType(val)
	if err != nil {
		return cty.NilVal, errors.WithStackTrace(err)
	}
	ctyOut, err := gocty.ToCtyValue(val, ctyType)
	if err != nil {
		return cty.NilVal, errors.WithStackTrace(err)
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
