// Package ctyhelper providers helpful tools for working with cty values.
package ctyhelper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
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

	if err := ValidateNumberRanges(unmarkedValue); err != nil {
		return nil, err
	}

	jsonBytes, err := ctyjson.Marshal(unmarkedValue, cty.DynamicPseudoType)
	if err != nil {
		return nil, err
	}

	var ctyJSONOutput CtyJSONOutput

	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.UseNumber()

	if err := decoder.Decode(&ctyJSONOutput); err != nil {
		return nil, err
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
		return cty.NilVal, err
	}

	return value, nil
}

// MaxNumberDecimalExponent is the largest power of ten a number's magnitude may reach, in
// either direction, before Terragrunt refuses to serialize it.
const MaxNumberDecimalExponent = 4096

// NumberOutOfRangeError reports a number that is too far from zero, or too close to it, for
// Terragrunt to serialize.
type NumberOutOfRangeError struct {
	Path cty.Path
}

func (err NumberOutOfRangeError) Error() string {
	msg := fmt.Sprintf(
		"number is outside the supported range of 1e-%d to 1e%d",
		MaxNumberDecimalExponent, MaxNumberDecimalExponent,
	)

	if attrPath := strings.TrimPrefix(ctyPathString(err.Path), "."); attrPath != "" {
		return attrPath + ": " + msg
	}

	return msg
}

// ValidateNumberRanges returns a [NumberOutOfRangeError] for the first number nested anywhere in
// value whose magnitude runs past [MaxNumberDecimalExponent] powers of ten in either direction.
//
// Numbers reach Terragrunt as arbitrary-precision floats, and writing one out in decimal costs far
// more time and memory than its digit count suggests. The literal 9E9999999 parses in microseconds
// and then takes minutes to render as a ten megabyte string, so callers check the range before
// serializing a value that came from a user.
func ValidateNumberRanges(value cty.Value) error {
	return cty.Walk(value, func(path cty.Path, val cty.Value) (bool, error) {
		if !val.Type().Equals(cty.Number) || val.IsNull() || !val.IsKnown() {
			return true, nil
		}

		unmarked, _ := val.Unmark()

		exp := unmarked.AsBigFloat().MantExp(nil)
		if (math.Abs(float64(exp))-1)*(math.Ln2/math.Ln10) > MaxNumberDecimalExponent {
			return false, NumberOutOfRangeError{Path: path.Copy()}
		}

		return true, nil
	})
}

func ctyPathString(path cty.Path) string {
	var b strings.Builder

	for _, step := range path {
		switch s := step.(type) {
		case cty.GetAttrStep:
			b.WriteString("." + s.Name)
		case cty.IndexStep:
			b.WriteString("[" + ctyIndexKeyString(s.Key) + "]")
		}
	}

	return b.String()
}

func ctyIndexKeyString(key cty.Value) string {
	const unrenderableKey = "*"

	if key.IsNull() || !key.IsKnown() {
		return unrenderableKey
	}

	switch {
	case key.Type().Equals(cty.String):
		return strconv.Quote(key.AsString())
	case key.Type().Equals(cty.Number):
		if idx, acc := key.AsBigFloat().Int64(); acc == big.Exact {
			return strconv.FormatInt(idx, 10)
		}
	}

	return unrenderableKey
}
