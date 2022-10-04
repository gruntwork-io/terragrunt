package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const defaultJSONOutName = "terragrunt_rendered.json"

const renderJsonHelp = `
   Usage: terragrunt render-json [OPTIONS]

   Description:
   Render the final terragrunt config, with all variables, includes, and functions resolved, as json.
   
   Options:
   --with-metadata 		Add metadata to the rendered JSON file.
   --terragrunt-json-out 	The file path that terragrunt should use when rendering the terragrunt.hcl config as json.
`

// runRenderJSON takes the parsed TerragruntConfig struct and renders it out as JSON so that it can be processed by
// other tools. To make it easier to maintain, this uses the cty representation as an intermediary.
// NOTE: An unspecified advantage of using the cty representation is that the final block outputs would be a map
// representation, which is easier to work with than the list representation that will be returned by a naive go-struct
// to json conversion.
func runRenderJSON(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntConfig == nil {
		return fmt.Errorf("Terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl.")
	}

	var terragruntConfigCty cty.Value

	if terragruntOptions.RenderJsonWithMetadata {
		cty, err := config.TerragruntConfigAsCtyWithMetadata(terragruntConfig)
		if err != nil {
			return err
		}
		terragruntConfigCty = cty
	} else {
		cty, err := config.TerragruntConfigAsCty(terragruntConfig)
		if err != nil {
			return err
		}
		terragruntConfigCty = cty
	}

	jsonBytes, err := marshalCtyValueJSONWithoutType(terragruntConfigCty)
	if err != nil {
		return err
	}

	jsonOutPath := terragruntOptions.JSONOut
	if jsonOutPath == "" {
		// Default to naming it `terragrunt_rendered.json` in the terragrunt config directory.
		terragruntConfigDir := filepath.Dir(terragruntOptions.TerragruntConfigPath)
		jsonOutPath = filepath.Join(terragruntConfigDir, defaultJSONOutName)
	}
	if err := util.EnsureDirectory(filepath.Dir(jsonOutPath)); err != nil {
		return err
	}
	terragruntOptions.Logger.Debugf("Rendering config %s to JSON %s", terragruntOptions.TerragruntConfigPath, jsonOutPath)

	if err := ioutil.WriteFile(jsonOutPath, jsonBytes, 0644); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// marshalCtyValueJSONWithoutType marshals the given cty.Value object into a JSON object that does not have the type.
// Using ctyjson directly would render a json object with two attributes, "value" and "type", and this function returns
// just the "value".
// NOTE: We have to do two marshalling passes so that we can extract just the value.
func marshalCtyValueJSONWithoutType(ctyVal cty.Value) ([]byte, error) {
	jsonBytesIntermediate, err := ctyjson.Marshal(ctyVal, cty.DynamicPseudoType)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	var ctyJsonOutput config.CtyJsonOutput
	if err := json.Unmarshal(jsonBytesIntermediate, &ctyJsonOutput); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	jsonBytes, err := json.Marshal(ctyJsonOutput.Value)
	return jsonBytes, errors.WithStackTrace(err)
}
