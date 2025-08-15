package ctyhelper_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

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

func TestParseCtyValueToMapWithInterpolationEscaping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected map[string]any
		input    cty.Value
		name     string
	}{
		{
			name: "map with interpolation gets escaped",
			input: cty.ObjectVal(map[string]cty.Value{
				"stuff": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("${bar}"),
				}),
			}),
			expected: map[string]any{
				"stuff": map[string]any{
					"foo": "$${bar}",
				},
			},
		},
		{
			name: "string with interpolation gets escaped",
			input: cty.ObjectVal(map[string]cty.Value{
				"simple": cty.StringVal("${var.example}"),
			}),
			expected: map[string]any{
				"simple": "$${var.example}",
			},
		},
		{
			name: "multiple interpolation patterns in same string",
			input: cty.ObjectVal(map[string]cty.Value{
				"key": cty.StringVal("${var.first} and ${var.second}"),
			}),
			expected: map[string]any{
				"key": "$${var.first} and $${var.second}",
			},
		},
		{
			name: "string with newlines and interpolation",
			input: cty.ObjectVal(map[string]cty.Value{
				"key": cty.StringVal("${bar}\n"),
			}),
			expected: map[string]any{
				"key": "$${bar}\n",
			},
		},
		{
			name: "no interpolation patterns",
			input: cty.ObjectVal(map[string]cty.Value{
				"key":  cty.StringVal("normal string"),
				"num":  cty.NumberIntVal(42),
				"bool": cty.BoolVal(true),
			}),
			expected: map[string]any{
				"key":  "normal string",
				"num":  float64(42), // JSON unmarshaling converts numbers to float64
				"bool": true,
			},
		},
		{
			name: "already escaped interpolation patterns remain unchanged",
			input: cty.ObjectVal(map[string]cty.Value{
				"already_escaped": cty.StringVal("$${var.example}"),
				"mixed":           cty.StringVal("$${escaped} and ${unescaped}"),
			}),
			expected: map[string]any{
				"already_escaped": "$${var.example}",
				"mixed":           "$${escaped} and $${unescaped}",
			},
		},
		{
			name: "nested map with already escaped patterns",
			input: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{
					"already_escaped": cty.StringVal("$${foo}"),
					"needs_escaping":  cty.StringVal("${bar}"),
				}),
			}),
			expected: map[string]any{
				"nested": map[string]any{
					"already_escaped": "$${foo}",
					"needs_escaping":  "$${bar}",
				},
			},
		},
		{
			name: "array with mixed escaped and unescaped patterns",
			input: cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{
					cty.StringVal("$${already_escaped}"),
					cty.StringVal("${needs_escaping}"),
					cty.StringVal("normal string"),
				}),
			}),
			expected: map[string]any{
				"items": []any{
					"$${already_escaped}",
					"$${needs_escaping}",
					"normal string",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test our function
			result, err := ctyhelper.ParseCtyValueToMap(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
