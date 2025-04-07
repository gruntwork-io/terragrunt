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

// PrintRawOutputs formats Terraform outputs for raw output format, similar to Terraform's output -raw.
// When the output is a raw output for a specific path, it will extract the raw value without quotes
// or formatting and write it directly to the provided writer.
// It only supports primitive values (strings, numbers, and booleans) and will return an error for complex types.
func PrintRawOutputs(_ *options.TerragruntOptions, writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	// Extract the value from the nested structure, if any
	valueMap := outputs.AsValueMap()

	var finalValue cty.Value

	// If there are multiple nested levels (from FilterOutputs), we need to extract the deepest value
	if len(valueMap) == 1 {
		// Get the first key-value pair (there's only one)
		var topKey string

		var topValue cty.Value

		for k, v := range valueMap {
			topKey = k
			topValue = v

			break
		}

		// Check if this is a nested structure containing only one path
		if topValue.Type().IsObjectType() {
			// Try to navigate to the leaf value by recursively going through the nested objects
			currentValue := topValue
			currentKey := topKey

			// Repeatedly traverse down, as long as we have a single-key object
			for currentValue.Type().IsObjectType() {
				nestedMap := currentValue.AsValueMap()
				if len(nestedMap) != 1 {
					// If we have more than one key at any level, we can't get a single raw value
					return errors.New("Error: Unsupported value for raw output\n\n" +
						"The -raw option only supports strings, numbers, and boolean values, but output value \"" + currentKey + "\" is " +
						currentValue.Type().FriendlyName() + ".\n\n" +
						"Use the -json option for machine-readable representations of output values that have complex types.")
				}

				// Get the only key-value pair in the nested object
				var nextKey string

				var nextValue cty.Value

				for k, v := range nestedMap {
					nextKey = k
					nextValue = v

					break
				}

				currentKey = nextKey
				currentValue = nextValue

				// If we've reached a primitive value, we're done
				if !currentValue.Type().IsObjectType() && !currentValue.Type().IsMapType() {
					finalValue = currentValue
					break
				}
			}

			// If we didn't set finalValue, the nested structure didn't lead to a primitive
			if finalValue == cty.NilVal {
				return errors.New("Error: Unsupported value for raw output\n\n" +
					"The -raw option only supports strings, numbers, and boolean values, but output value \"" + topKey + "\" is " +
					topValue.Type().FriendlyName() + ".\n\n" +
					"Use the -json option for machine-readable representations of output values that have complex types.")
			}
		} else {
			// Not a nested structure, just use the top-level value
			finalValue = topValue
		}
	} else if len(valueMap) > 1 {
		// Multiple top-level keys, can't provide a single raw output
		return errors.New("The -raw option requires a single output value. There are multiple outputs " +
			"available in the current stack. Please specify which output you want to display by using " +
			"the full output key as an argument to the command.")
	} else {
		// Empty map, nothing to output
		return nil
	}

	// Check if the final value is null
	if finalValue.IsNull() {
		return errors.New("Error: Unsupported value for raw output\n\n" +
			"The -raw option only supports strings, numbers, and boolean values, but the output value is null.\n\n" +
			"Use the -json option for machine-readable representations of output values that have complex types.")
	}

	// Check if the final value is a complex type
	if finalValue.Type().IsObjectType() || finalValue.Type().IsMapType() ||
		finalValue.Type().IsListType() || finalValue.Type().IsTupleType() ||
		finalValue.Type().IsSetType() {
		// Find the path to show in the error message
		var path string
		for k := range valueMap {
			path = k
			break
		}

		return errors.New("Error: Unsupported value for raw output\n\n" +
			"The -raw option only supports strings, numbers, and boolean values, but output value \"" + path + "\" is " +
			finalValue.Type().FriendlyName() + ".\n\n" +
			"Use the -json option for machine-readable representations of output values that have complex types.")
	}

	// Get string representation of the final value without quotes for raw output
	var valueStr string

	var err error

	// Unmark the value if it's marked (like with "sensitive")
	if finalValue.IsMarked() {
		finalValue, _ = finalValue.Unmark()
	}

	if finalValue.Type() == cty.String {
		// For strings, get the raw string value without quotes
		valueStr = finalValue.AsString()
	} else {
		// For other simple types, get their string representation
		valueStr, err = config.GetValueString(finalValue)
		if err != nil {
			return errors.New(err)
		}
	}

	// Write the raw value without any formatting
	if _, err := writer.Write([]byte(valueStr)); err != nil {
		return errors.New(err)
	}

	return nil
}

// PrintOutputs formats Terraform outputs as HCL and writes them to the provided writer.
// It creates a new HCL file with each top-level output as an attribute, preserving the
// original structure of complex types like maps and objects.
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

// PrintJSONOutput formats Terraform outputs as pretty-printed JSON with 2-space indentation.
// It marshals the cty.Value data to JSON using the go-cty library and writes the formatted
// result to the provided writer.
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
