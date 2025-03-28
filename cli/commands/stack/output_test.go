package stack_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/stack"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
)

func TestFilterOutputs(t *testing.T) {
	t.Parallel()
	outputs := map[string]map[string]cty.Value{
		"unit1": {
			"output1": cty.StringVal("value1"),
			"output2": cty.NumberIntVal(42),
		},
		"unit2": {
			"output3": cty.BoolVal(true),
			"nested": cty.ObjectVal(map[string]cty.Value{
				"inner": cty.StringVal("nested_value"),
			}),
		},
	}

	tests := []struct {
		name        string
		outputIndex string
		expectedKey string
		expectedLen int
		shouldExist bool
	}{
		{
			name:        "empty output index returns flattened map",
			outputIndex: "",
			expectedLen: 2,
			shouldExist: true,
		},
		{
			name:        "valid unit prefix returns filtered output",
			outputIndex: "unit1.output1",
			expectedLen: 1,
			shouldExist: true,
			expectedKey: "unit1.output1",
		},
		{
			name:        "invalid unit prefix returns nil",
			outputIndex: "unit3.output1",
			shouldExist: false,
		},
		{
			name:        "nested object access",
			outputIndex: "unit2.nested.inner",
			expectedLen: 1,
			shouldExist: true,
			expectedKey: "unit2.nested.inner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := stack.FilterOutputs(outputs, tt.outputIndex)

			if !tt.shouldExist {
				assert.Empty(t, result)
				return
			}

			assert.Len(t, result, tt.expectedLen)
			if tt.expectedKey != "" {
				_, exists := result[tt.expectedKey]
				assert.True(t, exists)
			}
		})
	}
}

func TestPrintJsonOutput(t *testing.T) {
	t.Parallel()
	outputs := map[string]map[string]cty.Value{
		"unit1": {
			"str": cty.StringVal("test"),
			"num": cty.NumberIntVal(123),
		},
	}

	tests := []struct {
		name        string
		outputIndex string
		expected    string
		shouldError bool
	}{
		{
			name:        "valid json output",
			outputIndex: "",
			expected:    `{"unit1":{"num":123,"str":"test"}}`,
			shouldError: false,
		},
		{
			name:        "filtered output",
			outputIndex: "unit1.str",
			expected:    `{"unit1.str":"test"}`,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := stack.PrintJSONOutput(&buf, outputs, tt.outputIndex)

			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Normalize the JSON for comparison
			var normalized map[string]any

			err = json.Unmarshal(buf.Bytes(), &normalized)

			require.NoError(t, err)

			expectedNormalized := make(map[string]any)
			err = json.Unmarshal([]byte(tt.expected), &expectedNormalized)
			require.NoError(t, err)

			assert.Equal(t, expectedNormalized, normalized)
		})
	}
}

// Mock implementation for testing
type mockWriter struct {
	err     error
	written []byte
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	m.written = append(m.written, p...)
	return len(p), nil
}

func TestPrintOutputs(t *testing.T) {
	t.Parallel()
	outputs := map[string]map[string]cty.Value{
		"unit1": {
			"output1": cty.StringVal("value1"),
			"output2": cty.NumberIntVal(42),
		},
	}

	tests := []struct {
		writerErr   error
		name        string
		outputIndex string
		shouldError bool
	}{
		{
			name:        "successful write",
			outputIndex: "",
			shouldError: false,
		},
		{
			name:        "writer error",
			outputIndex: "",
			writerErr:   assert.AnError,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			writer := &mockWriter{err: tt.writerErr}

			err := stack.PrintOutputs(writer, outputs, tt.outputIndex)

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, writer.written)
			}
		})
	}
}
