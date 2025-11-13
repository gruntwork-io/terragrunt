// Package ctyhelper providers helpful tools for working with cty values.
//
//nolint:dupl
package ctyhelper

import (
	"encoding/json"

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
//
// Note: This function will strip any marks (such as sensitive marks) from the values because JSON serialization does
// not support cty marks. If you need to preserve marks, consider working with cty.Value directly instead of converting
// to map[string]any.
func ParseCtyValueToMap(value cty.Value) (map[string]any, error) {
	if value.IsNull() {
		return map[string]any{}, nil
	}

	updatedValue, err := UpdateUnknownCtyValValues(value)
	if err != nil {
		return nil, err
	}

	value = updatedValue

	// Unmark the value (including nested values) before JSON serialization as JSON doesn't support marks.
	unmarkedValue, _ := value.UnmarkDeep()

	jsonBytes, err := ctyjson.Marshal(unmarkedValue, cty.DynamicPseudoType)
	if err != nil {
		return nil, errors.New(err)
	}

	var ctyJSONOutput CtyJSONOutput
	if err := json.Unmarshal(jsonBytes, &ctyJSONOutput); err != nil {
		return nil, errors.New(err)
	}

	return ctyJSONOutput.Value, nil
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
