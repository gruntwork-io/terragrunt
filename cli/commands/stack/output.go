package stack

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func PrintRawOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	if len(outputIndex) == 0 {
		// output index is required in raw mode
		return errors.New("output index is required in raw mode")
	}

	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	if len(filteredOutputs) > 1 {
		// return error since in raw mode we want to print only one output
		return errors.New("multiple outputs found, please specify only one index")
	}

	for key, value := range filteredOutputs {
		valueStr, err := getValueString(value)
		if err != nil {
			opts.Logger.Warnf("Error fetching output for '%s': %v", key, err)
			continue
		}

		line := valueStr + "\n"
		if _, err := writer.Write([]byte(line)); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func getValueString(value cty.Value) (string, error) {
	if value.Type() == cty.String {
		return value.AsString(), nil
	}

	return config.CtyValueAsString(value)
}

func PrintOutputs(writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	for key, value := range filteredOutputs {
		tokens := hclwrite.TokensForValue(value)
		rootBody.SetAttributeRaw(key, tokens)
	}

	if _, err := writer.Write(f.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func PrintJSONOutput(writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	topVal := cty.ObjectVal(filteredOutputs)
	rawJSON, err := ctyjson.Marshal(topVal, topVal.Type())

	if err != nil {
		return errors.New(err)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, rawJSON, "", "  "); err != nil {
		return errors.New(err)
	}

	if _, err := writer.Write(pretty.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func FilterOutputs(outputs map[string]map[string]cty.Value, outputIndex string) map[string]cty.Value {
	if outputIndex == "" {
		flattened := make(map[string]cty.Value)
		for unit, values := range outputs {
			flattened[unit] = cty.ObjectVal(values)
		}

		return flattened
	}

	keys := strings.Split(outputIndex, ".")
	currentMap := make(map[string]cty.Value)

	for unit, values := range outputs {
		if !strings.HasPrefix(outputIndex, unit) {
			continue
		}

		value := cty.ObjectVal(values)
		for _, key := range keys[1:] {
			if value.Type().IsObjectType() {
				mapVal := value.AsValueMap()
				if v, exists := mapVal[key]; exists {
					value = v
				} else {
					return nil
				}
			} else {
				return nil
			}
		}

		currentMap[outputIndex] = value
	}

	return currentMap
}
