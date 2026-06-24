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

func TestUpdateUnknownCtyValValuesTypedLeaves(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    cty.Value
		expected cty.Value
		name     string
	}{
		{
			name:     "unknown number leaf becomes zero",
			value:    cty.ObjectVal(map[string]cty.Value{"n": cty.UnknownVal(cty.Number)}),
			expected: cty.ObjectVal(map[string]cty.Value{"n": cty.Zero}),
		},
		{
			name:     "unknown bool leaf becomes false",
			value:    cty.ObjectVal(map[string]cty.Value{"b": cty.UnknownVal(cty.Bool)}),
			expected: cty.ObjectVal(map[string]cty.Value{"b": cty.False}),
		},
		{
			name:     "top level unknown number becomes zero",
			value:    cty.UnknownVal(cty.Number),
			expected: cty.Zero,
		},
		{
			name:     "unknown list leaf becomes empty list",
			value:    cty.ObjectVal(map[string]cty.Value{"items": cty.UnknownVal(cty.List(cty.String))}),
			expected: cty.ObjectVal(map[string]cty.Value{"items": cty.ListValEmpty(cty.String)}),
		},
		{
			name:     "unknown string leaf stays empty string",
			value:    cty.ObjectVal(map[string]cty.Value{"s": cty.UnknownVal(cty.String)}),
			expected: cty.ObjectVal(map[string]cty.Value{"s": cty.StringVal("")}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := ctyhelper.UpdateUnknownCtyValValues(tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
