package remote_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/remote"
)

func TestWrapMapToSingleLineHcl(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "SimpleMap",
			input:    map[string]interface{}{"key1": "value1", "key2": 46521694, "key3": true},
			expected: `{key1="value1",key2=46521694,key3=true}`,
		},
		{
			name:     "NestedMap",
			input:    map[string]interface{}{"key1": "value1", "key2": map[string]interface{}{"nestedKey": "nestedValue"}},
			expected: `{key1="value1",key2={nestedKey="nestedValue"}}`,
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := remote.WrapMapToSingleLineHcl(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, but got %s", tt.expected, result)
			}
		})
	}
}
