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

// runRenderJSON takes the parsed TerragruntConfig struct and renders it out as JSON so that it can be processed by
// other tools. To make it easier to maintain, this uses the cty representation as an intermediary.
// NOTE: An unspecified advantage of using the cty representation is that the final block outputs would be a map
// representation, which is easier to work with than the list representation that will be returned by a naive go-struct
// to json conversion.
func runRenderJSON(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntConfig == nil {
		return fmt.Errorf("Terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl.")
	}

	terragruntConfigCty, err := config.TerragruntConfigAsCty(terragruntConfig)
	if err != nil {
		return err
	}

	jsonBytes, err := marshalCtyValueJSONWithoutType(terragruntConfigCty)
	if err != nil {
		return err
	}

	jsonOutPath := terragruntOptions.JSONOut
	if err := util.EnsureDirectory(filepath.Dir(jsonOutPath)); err != nil {
		return err
	}

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
