package renderjson

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

func Run(opts *Options, terragruntConfig *config.TerragruntConfig) error {
	if terragruntConfig == nil {
		return fmt.Errorf("Terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl.")
	}

	var terragruntConfigCty cty.Value

	if opts.RenderJsonWithMetadata {
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

	jsonOutPath := opts.JSONOut
	if jsonOutPath == "" {
		// Default to naming it `terragrunt_rendered.json` in the terragrunt config directory.
		terragruntConfigDir := filepath.Dir(opts.TerragruntConfigPath)
		jsonOutPath = filepath.Join(terragruntConfigDir, defaultJSONOutName)
	}
	if err := util.EnsureDirectory(filepath.Dir(jsonOutPath)); err != nil {
		return err
	}
	opts.Logger.Debugf("Rendering config %s to JSON %s", opts.TerragruntConfigPath, jsonOutPath)

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
