package ctyhelper_test

import (
	"encoding/json"
	"fmt"
	"math/big"
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

func TestUpdateUnknownCtyValValuesWithSets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value         cty.Value
		expectedValue cty.Value
		name          string
	}{
		{
			name:          "set with unknown string elements",
			value:         cty.SetVal([]cty.Value{cty.UnknownVal(cty.String)}),
			expectedValue: cty.SetVal([]cty.Value{cty.StringVal("")}),
		},
		{
			name: "set with mixed known and unknown elements",
			value: cty.SetVal([]cty.Value{
				cty.StringVal("known-value"),
				cty.UnknownVal(cty.String),
			}),
			expectedValue: cty.SetVal([]cty.Value{
				cty.StringVal("known-value"),
				cty.StringVal(""),
			}),
		},
		{
			name:          "empty set",
			value:         cty.SetValEmpty(cty.String),
			expectedValue: cty.SetValEmpty(cty.String),
		},
		{
			name: "object containing a set with unknown elements",
			value: cty.ObjectVal(map[string]cty.Value{
				"my_test_variable": cty.SetVal([]cty.Value{
					cty.UnknownVal(cty.String),
					cty.UnknownVal(cty.String),
				}),
			}),
			expectedValue: cty.ObjectVal(map[string]cty.Value{
				"my_test_variable": cty.SetVal([]cty.Value{
					cty.StringVal(""),
				}),
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actualValue, err := ctyhelper.UpdateUnknownCtyValValues(tc.value)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedValue, actualValue)
		})
	}
}

func TestParseCtyValueToMapWithSets(t *testing.T) {
	t.Parallel()

	input := cty.ObjectVal(map[string]cty.Value{
		"my_test_variable": cty.SetVal([]cty.Value{
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
		}),
	})

	result, err := ctyhelper.ParseCtyValueToMap(input)
	require.NoError(t, err)
	require.Contains(t, result, "my_test_variable")
}
