package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

type TerragruntOutput struct {
	Name       string `hcl:",label"`
	ConfigPath string `hcl:"config_path,attr"`
}

// Decode the terragrunt_output blocks from the file, making sure to merge in those from included parents, and then
// retrieve all the outputs from the remote state. Then encode the resulting map as a cty.Value object.
func decodeAndRetrieveOutputs(
	file *hcl.File,
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (*cty.Value, error) {
	decodedTerragruntOutput := terragruntOutput{}
	err := decodeHcl(file, filename, &decodedTerragruntOutput, terragruntOptions, extensions)
	if err != nil {
		return nil, err
	}
	return terragruntOutputsToOutputMapCtyValue(decodedTerragruntOutput.TerragruntOutput, terragruntOptions)
}

// Convert the list of parsed TerragruntOutput blocks into a list of module dependencies. Each output block should
// become a dependency of the current config, since that module has to be applied before we can read the output.
func terragruntOutputsToModuleDependencies(decodedOutputBlocks []TerragruntOutput) *ModuleDependencies {
	paths := []string{}
	for _, decodedOutputBlock := range decodedOutputBlocks {
		configPath := decodedOutputBlock.ConfigPath
		if util.IsFile(configPath) && filepath.Base(configPath) == DefaultTerragruntConfigPath {
			// dependencies system expects the directory containing the terragrunt.hcl file
			configPath = filepath.Dir(configPath)
		}
		paths = append(paths, configPath)
	}
	return &ModuleDependencies{Paths: paths}
}

// Retrieve the output from the terraform state of each terragrunt_output block that was parsed. Encode the outputs as a
// map from the terragrunt_output block label to the corresponding output from the state. Finally, convert the map to
// cty.Value to a single cty.Value object, which is what is expected by the HCL2 decoder.
func terragruntOutputsToOutputMapCtyValue(outputConfigs []TerragruntOutput, terragruntOptions *options.TerragruntOptions) (*cty.Value, error) {
	paths := []string{}
	outputMap := map[string]cty.Value{}

	for _, outputConfig := range outputConfigs {
		paths = append(paths, outputConfig.ConfigPath)
		outputVal, err := getTerragruntOutput(outputConfig, terragruntOptions)
		if err != nil {
			return nil, err
		}
		if outputVal != nil {
			outputMap[outputConfig.Name] = *outputVal
		}
	}

	// We need to convert the value map to a single cty.Value at the end so that it can be used in the execution context
	convertedOutput, err := gocty.ToCtyValue(outputMap, generateTypeFromValuesMap(outputMap))
	if err != nil {
		err = TerragruntOutputListEncodingError{Paths: paths, Err: err}
	}
	return &convertedOutput, errors.WithStackTrace(err)
}

// Return the output from the state of another module, managed by terragrunt. This function will parse the provided
// terragrunt config and extract the desired output from the remote state. Note that this will error if the targetted
// module hasn't been applied yet.
func getTerragruntOutput(outputConfig TerragruntOutput, terragruntOptions *options.TerragruntOptions) (*cty.Value, error) {
	// target config check: make sure the target config exists
	cwd := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	targetConfig := outputConfig.ConfigPath
	if !filepath.IsAbs(targetConfig) {
		targetConfig = util.JoinPath(cwd, targetConfig)
	}
	if util.IsDir(targetConfig) {
		targetConfig = util.JoinPath(targetConfig, DefaultTerragruntConfigPath)
	}
	if !util.FileExists(targetConfig) {
		return nil, errors.WithStackTrace(TerragruntOutputConfigNotFound{Path: targetConfig})
	}

	jsonBytes, err := runTerragruntOutputJson(terragruntOptions, targetConfig)
	if err != nil {
		return nil, err
	}

	outputMap, err := terraformOutputJsonToCtyValueMap(targetConfig, jsonBytes)
	if err != nil {
		return nil, err
	}

	// We need to convert the value map to a single cty.Value at the end for use in the terragrunt config.
	convertedOutput, err := gocty.ToCtyValue(outputMap, generateTypeFromValuesMap(outputMap))
	if err != nil {
		err = TerragruntOutputEncodingError{Path: targetConfig, Err: err}
	}
	return &convertedOutput, errors.WithStackTrace(err)
}

// runTerragruntOutputJson uses terragrunt running functions to extract the json output from the target config. Make a
// copy of the terragruntOptions so that we can reuse the same execution environment.
func runTerragruntOutputJson(terragruntOptions *options.TerragruntOptions, targetConfig string) ([]byte, error) {
	var stdoutBuffer bytes.Buffer
	stdoutBufferWriter := bufio.NewWriter(&stdoutBuffer)
	targetOptions := terragruntOptions.Clone(targetConfig)
	targetOptions.TerraformCliArgs = []string{"output", "-json"}
	targetOptions.Writer = stdoutBufferWriter
	err := targetOptions.RunTerragrunt(targetOptions)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	stdoutBufferWriter.Flush()
	jsonString := stdoutBuffer.String()
	jsonBytes := []byte(jsonString)
	util.Debugf(terragruntOptions.Logger, "Retrieved output from %s as json: %s", targetConfig, jsonString)
	return jsonBytes, nil
}

// terraformOutputJsonToCtyValueMap takes the terraform output json and converts to a mapping between output keys to the
// parsed cty.Value encoding of the json objects.
func terraformOutputJsonToCtyValueMap(targetConfig string, jsonBytes []byte) (map[string]cty.Value, error) {
	// When getting all outputs, terraform returns a json with the data containing metadata about the types, so we
	// can't quite return the data directly. Instead, we will need further processing to get the output we want.
	// To do so, we first Unmarshal the json into a simple go map to a OutputMeta struct.
	type OutputMeta struct {
		Sensitive bool            `json:"sensitive"`
		Type      json.RawMessage `json:"type"`
		Value     json.RawMessage `json:"value"`
	}
	var outputs map[string]OutputMeta
	err := json.Unmarshal(jsonBytes, &outputs)
	if err != nil {
		return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
	}
	flattenedOutput := map[string]cty.Value{}
	for k, v := range outputs {
		outputType, err := ctyjson.UnmarshalType(v.Type)
		if err != nil {
			return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
		}
		outputVal, err := ctyjson.Unmarshal(v.Value, outputType)
		if err != nil {
			return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
		}
		flattenedOutput[k] = outputVal
	}
	return flattenedOutput, nil
}

// Custom error types

type TerragruntOutputConfigNotFound struct {
	Path string
}

func (err TerragruntOutputConfigNotFound) Error() string {
	return fmt.Sprintf("%s does not exist", err.Path)
}

type TerragruntOutputParsingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputParsingError) Error() string {
	return fmt.Sprintf("Could not parse output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputEncodingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputListEncodingError struct {
	Paths []string
	Err   error
}

func (err TerragruntOutputListEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from list of terragrunt configs %v. Underlying error: %s", err.Paths, err.Err)
}
