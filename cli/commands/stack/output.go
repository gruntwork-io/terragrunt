package stack

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"

	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func PrintRawOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	var buffer bytes.Buffer
	printValueMap(&buffer, "", outputs.AsValueMap())

	if _, err := writer.Write(buffer.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil

}

func getValueString(value cty.Value) (string, error) {
	if value.Type() == cty.String {
		return value.AsString(), nil
	}

	return config.CtyValueAsString(value)
}

func PrintOutputs(writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	for key, val := range outputs.AsValueMap() {
		rootBody.SetAttributeRaw(key, hclwrite.TokensForValue(val))
	}

	if _, err := writer.Write(f.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func printValueMap(buffer *bytes.Buffer, prefix string, valueMap map[string]cty.Value) {
	for key, val := range valueMap {
		newPrefix := key
		if prefix != "" {
			newPrefix = prefix + "." + key
		}

		if val.Type().IsObjectType() || val.Type().IsMapType() {
			// Recursively extract each field as a key
			for subKey, subVal := range val.AsValueMap() {
				subPrefix := newPrefix + "." + subKey
				if subVal.Type().IsObjectType() || subVal.Type().IsMapType() {
					printValueMap(buffer, subPrefix, subVal.AsValueMap())
				} else {
					valueStr, err := getValueString(subVal)
					if err != nil {
						continue
					}
					// Quote the value if it's a string
					if subVal.Type() == cty.String {
						buffer.WriteString(subPrefix + " = \"" + valueStr + "\"\n")
					} else {
						buffer.WriteString(subPrefix + " = " + valueStr + "\n")
					}
				}
			}
		} else {
			valueStr, err := getValueString(val)
			if err != nil {
				continue
			}
			// Quote the value if it's a string
			if val.Type() == cty.String {
				buffer.WriteString(newPrefix + " = \"" + valueStr + "\"\n")
			} else {
				buffer.WriteString(newPrefix + " = " + valueStr + "\n")
			}
		}
	}
}

func PrintJSONOutput(writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	rawJSON, err := ctyjson.Marshal(outputs, outputs.Type())

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
