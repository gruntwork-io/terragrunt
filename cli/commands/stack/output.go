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

// PrintRawOutputs formats Terraform outputs as flattened key-value pairs with dot notation for nested objects.
// It writes the formatted output to the provided writer, with string values enclosed in quotes and complex
// structures recursively traversed to create fully qualified paths for each value.
func PrintRawOutputs(_ *options.TerragruntOptions, writer io.Writer, outputs cty.Value) error {
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

// printValueMap recursively formats a map of cty.Values as key-value pairs with dot notation for nested paths.
// It handles complex nested structures by flattening them into fully-qualified paths (e.g., parent.child.key),
// writing the formatted output to the provided buffer. String values are automatically quoted, while other
// primitive types are written as-is. Each key-value pair is written on a separate line with an equals sign
// as the separator.
//
// Parameters:
//   - buffer: The bytes.Buffer where formatted output will be written
//   - prefix: The current path prefix to use (empty for top-level keys)
//   - valueMap: The map of cty.Values to format
//
// The function handles four main cases:
//  1. Top-level values (prefix is empty)
//  2. Nested values (prefix contains parent path)
//  3. Complex types (maps/objects) which require recursive traversal
//  4. Primitive values which are directly formatted
//
// Any errors encountered when converting values to strings are silently ignored,
// and those entries are skipped in the output.
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
					valueStr, err := config.GetValueString(subVal)
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
			valueStr, err := config.GetValueString(val)
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
