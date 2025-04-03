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

func PrintRawOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs cty.Value, outputIndex string) error {
	if len(outputIndex) == 0 {
		// output index is required in raw mode
		return errors.New("output index is required in raw mode")
	}

	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == cty.NilVal {
		return nil
	}

	valueMap := filteredOutputs.AsValueMap()

	if len(valueMap) > 1 {
		// return error since in raw mode we want to print only one output
		return errors.New("multiple outputs found, please specify only one index")
	}

	for key, value := range valueMap {
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

func PrintOutputs(writer io.Writer, outputs cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == cty.NilVal {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	for key, val := range filteredOutputs.AsValueMap() {
		rootBody.SetAttributeRaw(key, hclwrite.TokensForValue(val))
	}

	if _, err := writer.Write(f.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func PrintJSONOutput(writer io.Writer, outputs cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == cty.NilVal {
		return nil
	}

	rawJSON, err := ctyjson.Marshal(filteredOutputs, filteredOutputs.Type())

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

func FilterOutputs(outputs cty.Value, outputIndex string) cty.Value {
	if !outputs.IsKnown() || outputs.IsNull() || !outputs.Type().IsObjectType() {
		return cty.NilVal
	}

	if outputIndex == "" {
		return outputs
	}

	// Split the key path, e.g., "root_stack_1.app_3.data"
	keys := strings.Split(outputIndex, ".")
	current := outputs

	for _, key := range keys {
		if current.Type().IsObjectType() {
			valMap := current.AsValueMap()
			next, exists := valMap[key]
			if !exists {
				return cty.NilVal
			}
			current = next
		} else {
			return cty.NilVal
		}
	}

	return current
}
