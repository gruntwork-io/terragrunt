// Package ctyhelper providers helpful tools for working with cty values.
//
//nolint:dupl
package ctyhelper

import (
	"encoding/json"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ParseCtyValueToMap converts a cty.Value to a map[string]any.
//
// This is a hacky workaround to convert a cty Value to a Go map[string]any. cty does not support this directly
// (https://github.com/hashicorp/hcl2/issues/108) and doing it with gocty.FromCtyValue is nearly impossible, as cty
// requires you to specify all the output types and will error out when it hits interface{}. So, as an ugly workaround,
// we convert the given value to JSON using cty's JSON library and then convert the JSON back to a
// map[string]any using the Go json library.
func ParseCtyValueToMap(value cty.Value) (map[string]any, error) {
	if value.IsNull() {
		return map[string]any{}, nil
	}

	updatedValue, err := UpdateUnknownCtyValValues(value)
	if err != nil {
		return nil, err
	}

	value = updatedValue

	jsonBytes, err := ctyjson.Marshal(value, cty.DynamicPseudoType)
	if err != nil {
		return nil, errors.New(err)
	}

	var ctyJSONOutput CtyJSONOutput
	if err := json.Unmarshal(jsonBytes, &ctyJSONOutput); err != nil {
		return nil, errors.New(err)
	}

	// Escape interpolation patterns in the resulting map to prevent Terraform
	// from interpreting ${...} as variable references
	escapedOutput := escapeInterpolationPatterns(ctyJSONOutput.Value)

	return escapedOutput, nil
}

// escapeInterpolationPatterns recursively escapes ${...} patterns in all string values
// within a map structure to prevent Terraform from interpreting them as variable references
func escapeInterpolationPatterns(m map[string]any) map[string]any {
	result := make(map[string]any)

	for k, v := range m {
		switch val := v.(type) {
		case string:
			if strings.Contains(val, "${") {
				// Escape ${...} patterns by doubling the $: ${var} becomes $${var}
				// This prevents Terraform from interpreting them as variable interpolation
				escaped := strings.ReplaceAll(val, "${", "$${")
				result[k] = escaped
			} else {
				result[k] = val
			}
		case map[string]any:
			// Recursively escape nested maps
			result[k] = escapeInterpolationPatterns(val)
		case []any:
			// Handle arrays that might contain strings or maps
			result[k] = escapeInterpolationPatternsInSlice(val)
		default:
			// For all other types (numbers, booleans, etc.), keep as-is
			result[k] = val
		}
	}

	return result
}

// escapeInterpolationPatternsInSlice handles arrays that might contain strings or maps
func escapeInterpolationPatternsInSlice(slice []any) []any {
	result := make([]any, len(slice))

	for i, v := range slice {
		switch val := v.(type) {
		case string:
			if strings.Contains(val, "${") {
				// Escape ${...} patterns by doubling the $: ${var} becomes $${var}
				escaped := strings.ReplaceAll(val, "${", "$${")
				result[i] = escaped
			} else {
				result[i] = val
			}
		case map[string]any:
			result[i] = escapeInterpolationPatterns(val)
		case []any:
			result[i] = escapeInterpolationPatternsInSlice(val)
		default:
			result[i] = val
		}
	}

	return result
}

// CtyJSONOutput is a struct that captures the output of cty's JSON marshalling.
//
// When you convert a cty value to JSON, if any of that types are not yet known (i.e., are labeled as
// DynamicPseudoType), cty's Marshall method will write the type information to a type field and the actual value to
// a value field. This struct is used to capture that information so when we parse the JSON back into a Go struct, we
// can pull out just the Value field we need.
type CtyJSONOutput struct {
	Value map[string]any `json:"Value"`
	Type  any            `json:"Type"`
}

// UpdateUnknownCtyValValues deeply updates unknown values with default value
func UpdateUnknownCtyValValues(value cty.Value) (cty.Value, error) {
	var updatedValue any

	switch {
	case !value.IsKnown():
		return cty.StringVal(""), nil
	case value.IsNull():
		return value, nil
	case value.Type().IsMapType(), value.Type().IsObjectType():
		mapVals := value.AsValueMap()
		for key, val := range mapVals {
			val, err := UpdateUnknownCtyValValues(val)
			if err != nil {
				return cty.NilVal, err
			}

			mapVals[key] = val
		}

		if len(mapVals) > 0 {
			updatedValue = mapVals
		}

	case value.Type().IsTupleType(), value.Type().IsListType():
		sliceVals := value.AsValueSlice()
		for key, val := range sliceVals {
			val, err := UpdateUnknownCtyValValues(val)
			if err != nil {
				return cty.NilVal, err
			}

			sliceVals[key] = val
		}

		if len(sliceVals) > 0 {
			updatedValue = sliceVals
		}
	}

	if updatedValue == nil {
		return value, nil
	}

	value, err := gocty.ToCtyValue(updatedValue, value.Type())
	if err != nil {
		return cty.NilVal, errors.New(err)
	}

	return value, nil
}
