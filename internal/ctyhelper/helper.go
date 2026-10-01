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
	"unicode/utf8"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// ParseCtyValueToMap converts a cty.Value to a map[string]any.
//
// The result is what encoding the value with cty's JSON library and decoding it with a [json.Decoder] that has
// [json.Decoder.UseNumber] set would produce: objects and maps become map[string]any, lists, sets and tuples become
// []any, numbers become [json.Number], and nulls become nil. Unknown values become empty strings.
//
// Note: This function will strip any marks (such as sensitive marks) from the values because JSON serialization does
// not support cty marks. If you need to preserve marks, consider working with cty.Value directly instead of converting
// to map[string]any.
func ParseCtyValueToMap(value cty.Value) (map[string]any, error) {
	if value.IsNull() {
		return map[string]any{}, nil
	}

	value, _ = value.UnmarkDeep()

	if ty := value.Type(); ty.IsObjectType() || ty.IsMapType() {
		if out, ok := mappingToGo(value); ok {
			return out, nil
		}
	}

	return parseCtyValueToMapWithJSON(value)
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

		if !numberInRange(unmarked.AsBigFloat()) {
			return false, NumberOutOfRangeError{Path: path.Copy()}
		}

		return true, nil
	})
}

// parseCtyValueToMapWithJSON converts an unmarked value by encoding it with cty's JSON library and decoding the
// result. It handles the values mappingToGo declines, and its errors are the ones [ParseCtyValueToMap] reports.
func parseCtyValueToMapWithJSON(value cty.Value) (map[string]any, error) {
	value, err := UpdateUnknownCtyValValues(value)
	if err != nil {
		return nil, err
	}

	if err := ValidateNumberRanges(value); err != nil {
		return nil, err
	}

	jsonBytes, err := ctyjson.Marshal(value, cty.DynamicPseudoType)
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

// mappingToGo converts an unmarked object or map to the result parseCtyValueToMapWithJSON would give for it.
//
// Reports false when the value is or holds anything that conversion would change or reject: an unknown value, an
// infinity, a number outside [MaxNumberDecimalExponent], a string that is not valid UTF-8, or a capsule.
func mappingToGo(val cty.Value) (map[string]any, bool) {
	if !val.IsKnown() {
		return nil, false
	}

	out := make(map[string]any, val.LengthInt())

	for it := val.ElementIterator(); it.Next(); {
		k, ev := it.Element()

		key := k.AsString()
		if !utf8.ValidString(key) {
			return nil, false
		}

		goVal, ok := ctyValueToGo(ev)
		if !ok {
			return nil, false
		}

		out[key] = goVal
	}

	return out, true
}

// ctyValueToGo converts one unmarked value nested in a value passed to mappingToGo, reporting false on the same
// values it does.
func ctyValueToGo(val cty.Value) (any, bool) {
	if !val.IsKnown() {
		return nil, false
	}

	if val.IsNull() {
		return nil, true
	}

	ty := val.Type()

	switch {
	case ty == cty.String:
		s := val.AsString()

		return s, utf8.ValidString(s)
	case ty == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInf() || !numberInRange(bf) {
			return nil, false
		}

		return json.Number(bf.Text('f', -1)), true
	case ty == cty.Bool:
		return val.True(), true
	case ty.IsObjectType(), ty.IsMapType():
		return mappingToGo(val)
	case ty.IsListType(), ty.IsSetType(), ty.IsTupleType():
		return sequenceToGo(val)
	}

	return nil, false
}

// sequenceToGo converts an unmarked list, set, or tuple, reporting false on the same values mappingToGo does.
func sequenceToGo(val cty.Value) ([]any, bool) {
	out := make([]any, 0, val.LengthInt())

	for it := val.ElementIterator(); it.Next(); {
		_, ev := it.Element()

		goVal, ok := ctyValueToGo(ev)
		if !ok {
			return nil, false
		}

		out = append(out, goVal)
	}

	return out, true
}

// numberInRange reports whether the magnitude of n stays within [MaxNumberDecimalExponent] powers of ten in either
// direction.
func numberInRange(n *big.Float) bool {
	exp := n.MantExp(nil)

	return (math.Abs(float64(exp))-1)*(math.Ln2/math.Ln10) <= MaxNumberDecimalExponent
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
