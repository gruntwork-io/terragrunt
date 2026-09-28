package ctyhelper_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
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
