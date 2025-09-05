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

// PrintRawOutputs formats  outputs for raw output format, similar to Tofu's output -raw.
// When the output is a raw output for a specific path, it will extract the raw value without quotes
// or formatting and write it directly to the provided writer.
// It only supports primitive values (strings, numbers, and booleans) and will return an error for complex types.
func PrintRawOutputs(_ *options.TerragruntOptions, writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	// Extract the value from the nested structure, if any
	valueMap := outputs.AsValueMap()

	length := len(valueMap)

	if length == 0 {
		return nil
	}

	if length == 1 {
		// Single output, try to extract the final value
		finalValue, err := extractSingleValue(valueMap)
		if err != nil {
			return err
		}

		return writePrimitiveValue(writer, finalValue, config.GetFirstKey(valueMap))
	}

	// Multiple top-level keys, can't provide a single raw output
	return errors.New("The -raw option requires a single output value. There are multiple outputs " +
		"available in the current stack. Please specify which output you want to display by using " +
		"the full output key as an argument to the command.")
}

// extractSingleValue extracts a single primitive value from a map with only one element,
// potentially traversing through a nested object structure.
func extractSingleValue(valueMap map[string]cty.Value) (cty.Value, error) {
	topKey := config.GetFirstKey(valueMap)
	topValue := valueMap[topKey]

	// If the value is not an object type, return it directly
	if !topValue.Type().IsObjectType() {
		return topValue, nil
	}

	// Try to navigate to the leaf value through nested objects
	return traverseNestedObject(topKey, topValue)
}

// traverseNestedObject follows a chain of nested objects to find a primitive value at the leaf.
// Returns an error if a complex value is found at the leaf or if multiple paths are present.
func traverseNestedObject(topKey string, topValue cty.Value) (cty.Value, error) {
	currentValue := topValue
	currentKey := topKey

	var finalValue cty.Value

	// Traverse down the nested objects
	for currentValue.Type().IsObjectType() {
		nestedMap := currentValue.AsValueMap()
		if len(nestedMap) != 1 {
			// If we have more than one key at any level, we can't get a single raw value
			return cty.NilVal, createUnsupportedValueError(currentKey, currentValue)
		}

		// Get the only key-value pair in the nested object
		nextKey := config.GetFirstKey(nestedMap)
		nextValue := nestedMap[nextKey]

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
		return cty.NilVal, createUnsupportedValueError(topKey, topValue)
	}

	return finalValue, nil
}

// writePrimitiveValue writes a primitive value to the writer.
// Returns an error if the value is null or a complex type.
func writePrimitiveValue(writer io.Writer, value cty.Value, path string) error {
	// Check if the value is null
	if value.IsNull() {
		return errors.New("Error: Unsupported value for raw output\n\n" +
			"The -raw option only supports strings, numbers, and boolean values, but the output value is null.\n\n" +
			"Use the -json option for machine-readable representations of output values that have complex types.")
	}

	// Check if the value is a complex type
	if config.IsComplexType(value) {
		return createUnsupportedValueError(path, value)
	}

	// Unmark the value if it's marked (like with "sensitive")
	if value.IsMarked() {
		value, _ = value.Unmark()
	}

	valueStr, err := config.FormatValue(value)
	if err != nil {
		return errors.New(err)
	}

	// Write the raw value without any formatting
	if _, err := writer.Write([]byte(valueStr)); err != nil {
		return errors.New(err)
	}

	return nil
}

// createUnsupportedValueError creates a formatted error for unsupported value types.
func createUnsupportedValueError(path string, value cty.Value) error {
	return errors.New("Error: Unsupported value for raw output\n\n" +
		"The -raw option only supports strings, numbers, and boolean values, but output value \"" + path + "\" is " +
		value.Type().FriendlyName() + ".\n\n" +
		"Use the -json option for machine-readable representations of output values that have complex types.")
}

// PrintOutputs formats outputs as HCL and writes them to the provided writer.
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

// PrintJSONOutput formats outputs as pretty-printed JSON with 2-space indentation.
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
