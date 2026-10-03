package ctyhelper_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func TestParseCtyValueToMapPreservesLargeNumberPrecision(t *testing.T) {
	t.Parallel()

	// Reproduces https://github.com/gruntwork-io/terragrunt/issues/3514
	// Large integers (>16 digits) lost precision because json.Unmarshal
	// decoded them as float64.
	largeNumber := "111111111111111111"
	bigFloat, _, _ := big.ParseFloat(largeNumber, 10, 512, big.ToNearestEven)

	input := cty.ObjectVal(map[string]cty.Value{
		"some_number": cty.NumberVal(bigFloat),
	})

	result, err := ctyhelper.ParseCtyValueToMap(input)
	require.NoError(t, err)

	// The value should be a json.Number preserving full precision, not a float64.
	num, ok := result["some_number"].(json.Number)
	require.True(t, ok, "expected json.Number, got %T", result["some_number"])
	assert.Equal(t, largeNumber, num.String(),
		"large number should survive the cty→map round trip without precision loss")
}

func TestUpdateUnknownCtyValValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value         cty.Value
		expectedValue cty.Value
	}{
		{
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.UnknownVal(cty.String),
				})}),
			})}),
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.StringVal(""),
				})}),
			})}),
		},
		{
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{}),
			cty.ObjectVal(map[string]cty.Value{}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{"key": cty.UnknownVal(cty.String)}),
			cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("")}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualValue, err := ctyhelper.UpdateUnknownCtyValValues(tc.value)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedValue, actualValue)
		})
	}
}

func TestValidateNumberRanges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr *ctyhelper.NumberOutOfRangeError
		value       cty.Value
		name        string
	}{
		{
			name:  "largest float64",
			value: cty.MustParseNumberVal("1.7976931348623157E308"),
		},
		{
			name:  "smallest normal float64",
			value: cty.MustParseNumberVal("2.2250738585072014E-308"),
		},
		{
			name:  "eighteen digit integer",
			value: cty.MustParseNumberVal("111111111111111111"),
		},
		{
			name:  "largest supported magnitude",
			value: cty.MustParseNumberVal("1E4096"),
		},
		{
			name:  "smallest supported magnitude",
			value: cty.MustParseNumberVal("1E-4096"),
		},
		{
			name:        "just past the largest supported magnitude",
			value:       cty.MustParseNumberVal("1E4097"),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.Path{}},
		},
		{
			name:        "just past the smallest supported magnitude",
			value:       cty.MustParseNumberVal("1E-4097"),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.Path{}},
		},
		{
			name:  "zero",
			value: cty.Zero,
		},
		{
			name: "null and unknown numbers",
			value: cty.ObjectVal(map[string]cty.Value{
				"unknown": cty.UnknownVal(cty.Number),
				"null":    cty.NullVal(cty.Number),
			}),
		},
		{
			name:        "top level number",
			value:       cty.MustParseNumberVal("9E9999999"),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.Path{}},
		},
		{
			name: "exponent far above the cap",
			value: cty.ObjectVal(map[string]cty.Value{
				"count": cty.MustParseNumberVal("9E9999999"),
			}),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.GetAttrPath("count")},
		},
		{
			name: "exponent far below the cap",
			value: cty.ObjectVal(map[string]cty.Value{
				"count": cty.MustParseNumberVal("9E-9999999"),
			}),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.GetAttrPath("count")},
		},
		{
			name: "nested in a list",
			value: cty.ObjectVal(map[string]cty.Value{
				"sizes": cty.ListVal([]cty.Value{
					cty.NumberIntVal(1),
					cty.MustParseNumberVal("9E9999999"),
				}),
			}),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.GetAttrPath("sizes").IndexInt(1)},
		},
		{
			name: "marked",
			value: cty.ObjectVal(map[string]cty.Value{
				"secret": cty.MustParseNumberVal("9E9999999").Mark("sensitive"),
			}),
			expectedErr: &ctyhelper.NumberOutOfRangeError{Path: cty.GetAttrPath("secret")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ctyhelper.ValidateNumberRanges(tc.value)

			if tc.expectedErr == nil {
				require.NoError(t, err)

				return
			}

			var rangeErr ctyhelper.NumberOutOfRangeError

			require.ErrorAs(t, err, &rangeErr)
			assert.Equal(t, tc.expectedErr.Path, rangeErr.Path)
		})
	}
}

func TestParseCtyValueToMapKeepsLargeNumbersWhole(t *testing.T) {
	t.Parallel()

	value := cty.ObjectVal(map[string]cty.Value{
		"count": cty.MustParseNumberVal("9E4000"),
	})

	result, err := ctyhelper.ParseCtyValueToMap(value)
	require.NoError(t, err)
	assert.Equal(t, json.Number("9"+strings.Repeat("0", 4000)), result["count"])
}

func TestParseCtyValueToMapMatchesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value cty.Value
		name  string
	}{
		{
			name: "primitives",
			value: cty.ObjectVal(map[string]cty.Value{
				"string": cty.StringVal("a <b> &   \"c\""),
				"int":    cty.NumberIntVal(-42),
				"float":  cty.NumberFloatVal(1.5),
				"zero":   cty.Zero,
				"true":   cty.True,
				"false":  cty.False,
			}),
		},
		{
			name: "large and small numbers",
			value: cty.ObjectVal(map[string]cty.Value{
				"big":      cty.MustParseNumberVal("111111111111111111"),
				"huge":     cty.MustParseNumberVal("9E4000"),
				"tiny":     cty.MustParseNumberVal("1E-4096"),
				"fraction": cty.MustParseNumberVal("0.1"),
			}),
		},
		{
			name: "nulls",
			value: cty.ObjectVal(map[string]cty.Value{
				"string":  cty.NullVal(cty.String),
				"dynamic": cty.NullVal(cty.DynamicPseudoType),
				"list":    cty.NullVal(cty.List(cty.String)),
				"object":  cty.NullVal(cty.EmptyObject),
			}),
		},
		{
			name: "collections",
			value: cty.ObjectVal(map[string]cty.Value{
				"list":        cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
				"set":         cty.SetVal([]cty.Value{cty.StringVal("b"), cty.StringVal("a")}),
				"tuple":       cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.NumberIntVal(1), cty.True}),
				"map":         cty.MapVal(map[string]cty.Value{"k": cty.StringVal("v")}),
				"empty_list":  cty.ListValEmpty(cty.String),
				"empty_set":   cty.SetValEmpty(cty.Number),
				"empty_tuple": cty.EmptyTupleVal,
				"empty_map":   cty.MapValEmpty(cty.DynamicPseudoType),
				"empty_obj":   cty.EmptyObjectVal,
			}),
		},
		{
			name: "nesting",
			value: cty.ObjectVal(map[string]cty.Value{
				"accounts": cty.MapVal(map[string]cty.Value{
					"dev": cty.ObjectVal(map[string]cty.Value{
						"id":      cty.StringVal("111111111111"),
						"regions": cty.ListVal([]cty.Value{cty.StringVal("us-east-1")}),
						"tags":    cty.MapValEmpty(cty.String),
					}),
					"prod": cty.ObjectVal(map[string]cty.Value{
						"id":      cty.StringVal("222222222222"),
						"regions": cty.ListVal([]cty.Value{cty.StringVal("eu-west-1")}),
						"tags":    cty.MapVal(map[string]cty.Value{"tier": cty.StringVal("1")}),
					}),
				}),
			}),
		},
		{
			name:  "map at the top level",
			value: cty.MapVal(map[string]cty.Value{"a": cty.NumberIntVal(1), "b": cty.NumberIntVal(2)}),
		},
		{
			name: "unknowns",
			value: cty.ObjectVal(map[string]cty.Value{
				"string": cty.UnknownVal(cty.String),
				"nested": cty.ObjectVal(map[string]cty.Value{"list": cty.ListVal([]cty.Value{cty.UnknownVal(cty.String)})}),
			}),
		},
		{
			name:  "unknown object at the top level",
			value: cty.UnknownVal(cty.Object(map[string]cty.Type{"enabled": cty.Bool})),
		},
		{
			name:  "unknown map at the top level",
			value: cty.UnknownVal(cty.Map(cty.String)),
		},
		{
			name: "nested marks",
			value: cty.ObjectVal(map[string]cty.Value{
				"secret": cty.StringVal("hunter2").Mark("sensitive"),
				"list":   cty.ListVal([]cty.Value{cty.NumberIntVal(1).Mark("sensitive")}),
			}),
		},
		{
			name:  "infinity",
			value: cty.ObjectVal(map[string]cty.Value{"inf": cty.PositiveInfinity}),
		},
		{
			name:  "exponent past the cap",
			value: cty.ObjectVal(map[string]cty.Value{"n": cty.MustParseNumberVal("1E4097")}),
		},
		{
			name:  "invalid UTF-8",
			value: cty.ObjectVal(map[string]cty.Value{"s": cty.StringVal("a\xffb")}),
		},
		{
			name:  "invalid UTF-8 key",
			value: cty.MapVal(map[string]cty.Value{"a\xffb": cty.True}),
		},
		{
			name:  "string at the top level",
			value: cty.StringVal("value"),
		},
		{
			name:  "list at the top level",
			value: cty.ListVal([]cty.Value{cty.StringVal("value")}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expected, expectedErr := jsonRoundTrip(tc.value)

			actual, err := ctyhelper.ParseCtyValueToMap(tc.value)

			assert.Equal(t, expectedErr, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestParseCtyValueToMapUnmarksTopLevelValue(t *testing.T) {
	t.Parallel()

	value := cty.ObjectVal(map[string]cty.Value{"secret": cty.StringVal("hunter2")}).Mark("sensitive")

	result, err := ctyhelper.ParseCtyValueToMap(value)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"secret": "hunter2"}, result)
}

// FuzzParseCtyValueToMap pins that converting any value decoded from JSON gives
// the same result, or the same error, as a round trip through cty's JSON
// encoding.
func FuzzParseCtyValueToMap(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"a":"b","n":1.5,"t":true,"z":null}`,
		`{"big":111111111111111111,"neg":-0,"exp":1e300}`,
		`{"huge":1e99999}`,
		`{"list":[1,"a",[],{}],"obj":{"nested":{"deeper":[null]}}}`,
		`{"escapes":"<>& \u0000"}`,
		`[1,2]`,
		`"string"`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		ty, err := ctyjson.ImpliedType([]byte(input))
		if err != nil {
			return
		}

		value, err := ctyjson.Unmarshal([]byte(input), ty)
		if err != nil {
			return
		}

		expected, expectedErr := jsonRoundTrip(value)

		actual, err := ctyhelper.ParseCtyValueToMap(value)

		assert.Equal(t, expectedErr, err)
		assert.Equal(t, expected, actual)
	})
}

// jsonRoundTrip converts value through cty's JSON encoding and back, the
// conversion ParseCtyValueToMap has to match.
func jsonRoundTrip(value cty.Value) (map[string]any, error) {
	if value.IsNull() {
		return map[string]any{}, nil
	}

	value, _ = value.UnmarkDeep()

	value, err := ctyhelper.UpdateUnknownCtyValValues(value)
	if err != nil {
		return nil, err
	}

	if err := ctyhelper.ValidateNumberRanges(value); err != nil {
		return nil, err
	}

	jsonBytes, err := ctyjson.Marshal(value, cty.DynamicPseudoType)
	if err != nil {
		return nil, err
	}

	var out ctyhelper.CtyJSONOutput

	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.UseNumber()

	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}

	return out.Value, nil
}

func TestParseCtyValueToMapRejectsExtremeExponents(t *testing.T) {
	t.Parallel()

	value := cty.ObjectVal(map[string]cty.Value{
		"count": cty.MustParseNumberVal("9E9999999"),
	})

	_, err := ctyhelper.ParseCtyValueToMap(value)

	var rangeErr ctyhelper.NumberOutOfRangeError

	require.ErrorAs(t, err, &rangeErr)
	assert.Equal(t, cty.GetAttrPath("count"), rangeErr.Path)
}
