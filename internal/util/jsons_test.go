package util_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsTerraformEnvVarJsonValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    any
		expected string
	}{
		// plain strings: passed through unchanged (Terraform reads string vars as literals)
		{"aws_region", "aws_region"},
		{"plain ${bar} string", "plain ${bar} string"},
		// list: JSON serialized, strings within escaped
		{[]string{"10.0.0.0/16", "10.0.0.10/16"}, "[\"10.0.0.0/16\",\"10.0.0.10/16\"]"},
		// map: strings within escaped
		{map[string]any{"foo": "test ${bar} test"}, `{"foo":"test $${bar} test"}`},
		// idempotent: already-escaped $${...} not double-escaped
		{map[string]any{"foo": "test $${bar} test"}, `{"foo":"test $${bar} test"}`},
		// list with interpolation
		{[]any{"${foo}", "bar"}, `["$${foo}","bar"]`},
		// nested map
		{map[string]any{"a": map[string]any{"b": "${nested}"}}, `{"a":{"b":"$${nested}"}}`},
		// typed []string with interpolation
		{[]string{"${foo}", "bar"}, `["$${foo}","bar"]`},
		// typed map[string]string with interpolation
		{map[string]string{"k": "${foo}"}, `{"k":"$${foo}"}`},
		// nil containers must serialize as null, not {} or []
		{(map[string]any)(nil), "null"},
		{([]any)(nil), "null"},
		{([]string)(nil), "null"},
		{(map[string]string)(nil), "null"},
		// nil inside a complex type must also be null
		{map[string]any{"list": ([]any)(nil)}, `{"list":null}`},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.AsTerraformEnvVarJSONValue(tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestAsTerraformEnvVarJsonValueDepthOverflow(t *testing.T) {
	t.Parallel()

	// Build a map nested 102 levels deep — exceeds the maxDepth of 100.
	deep := buildNestedMap(102)

	_, err := util.AsTerraformEnvVarJSONValue(deep)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum nesting depth")
}

// buildNestedMap creates a map[string]any with the given nesting depth.
func buildNestedMap(depth int) map[string]any {
	if depth == 0 {
		return map[string]any{"val": "leaf"}
	}

	return map[string]any{"nested": buildNestedMap(depth - 1)}
}

func TestEscapeInterpolationInString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected string
	}{
		{"test ${bar} test", "test $${bar} test"},
		// idempotent: already escaped
		{"test $${bar} test", "test $${bar} test"},
		// multiple interpolations
		{"${a} and ${b}", "$${a} and $${b}"},
		// no interpolation
		{"no interpolation", "no interpolation"},
		// dollar not followed by brace
		{"$not_interpolation", "$not_interpolation"},
		// empty string
		{"", ""},
		// just the pattern
		{"${foo}", "$${foo}"},
		// starts with already-escaped
		{"$${foo} and ${bar}", "$${foo} and $${bar}"},
	}

	for i, tc := range testCases {
		name := tc.input
		if name == "" {
			name = fmt.Sprintf("case-%d", i)
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			actual := util.EscapeInterpolationInString(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
