package hclhelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
)

func TestWrapMapToSingleLineHcl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    map[string]any
		expected string
	}{
		{
			name:     "SimpleMap",
			input:    map[string]any{"key1": "value1", "key2": 46521694, "key3": true},
			expected: `{key1="value1",key2=46521694,key3=true}`,
		},
		{
			name:     "NestedMap",
			input:    map[string]any{"key1": "value1", "key2": map[string]any{"nestedKey": "nestedValue"}},
			expected: `{key1="value1",key2={nestedKey="nestedValue"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := hclhelper.WrapMapToSingleLineHcl(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, result)
			}
		})
	}
}
