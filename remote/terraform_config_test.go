package remote

import "testing"

func TestWrapMapToSingleLineHcl(t *testing.T) {
	tests := []struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapMapToSingleLineHcl(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, but got %s", tt.expected, result)
			}
		})
	}
}
