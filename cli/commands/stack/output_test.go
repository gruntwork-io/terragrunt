package stack_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/cli/commands/stack"

	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
)

func TestPrintRawOutputs(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.StringVal("value1"),
		"key2": cty.NumberIntVal(2),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Contains(t, buffer.String(), "key1 = \"value1\"")
	assert.Contains(t, buffer.String(), "key2 = 2")
}

func TestPrintOutputs(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.StringVal("value1"),
		"key2": cty.NumberIntVal(2),
	})

	err := stack.PrintOutputs(&buffer, outputs)
	require.NoError(t, err)
	assert.Contains(t, buffer.String(), "key1 = \"value1\"")
	assert.Contains(t, buffer.String(), "key2 = 2")
}

func TestPrintJSONOutput(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.StringVal("value1"),
		"key2": cty.NumberIntVal(2),
	})

	err := stack.PrintJSONOutput(&buffer, outputs)
	require.NoError(t, err)
	assert.JSONEq(t, `{"key1":"value1","key2":2}`, buffer.String())
}

func TestPrintRawOutputsEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		outputs  cty.Value
		expected []string
	}{
		{
			name:     "Empty Outputs",
			outputs:  cty.ObjectVal(map[string]cty.Value{}),
			expected: []string{},
		},
		{
			name:     "Nil Outputs",
			outputs:  cty.NilVal,
			expected: []string{},
		},
		{
			name: "Nested Structures",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"parent": cty.ObjectVal(map[string]cty.Value{
					"child": cty.StringVal("value"),
				}),
			}),
			expected: []string{"parent.child = \"value\""},
		},
		{
			name: "Different Data Types",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"string": cty.StringVal("text"),
				"number": cty.NumberIntVal(42),
				"bool":   cty.BoolVal(true),
				"list":   cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
			expected: []string{
				"string = \"text\"",
				"number = 42",
				"bool = true",
				"list = [\"a\",\"b\"]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			err := stack.PrintRawOutputs(nil, &buffer, tt.outputs)
			require.NoError(t, err)
			output := buffer.String()
			for _, expectedLine := range tt.expected {
				assert.Contains(t, output, expectedLine)
			}
		})
	}
}

func TestPrintOutputsEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		outputs  cty.Value
		expected []string
	}{
		{
			name:     "Empty Outputs",
			outputs:  cty.ObjectVal(map[string]cty.Value{}),
			expected: []string{},
		},
		{
			name:     "Nil Outputs",
			outputs:  cty.NilVal,
			expected: []string{},
		},
		{
			name: "Nested Structures",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"parent": cty.ObjectVal(map[string]cty.Value{
					"child": cty.StringVal("value"),
				}),
			}),
			expected: []string{"parent = {", "child = \"value\""},
		},
		{
			name: "Different Data Types",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"string": cty.StringVal("text"),
				"number": cty.NumberIntVal(42),
				"bool":   cty.BoolVal(true),
				"list":   cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
			expected: []string{
				"string = \"text\"",
				"number = 42",
				"bool   = true",
				"list   = [",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			err := stack.PrintOutputs(&buffer, tt.outputs)
			require.NoError(t, err)
			output := buffer.String()
			for _, expectedLine := range tt.expected {
				assert.Contains(t, output, expectedLine)
			}
		})
	}
}

func TestPrintJSONOutputEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		outputs  cty.Value
		expected string
		isNil    bool
	}{
		{
			name:     "Empty Outputs",
			outputs:  cty.ObjectVal(map[string]cty.Value{}),
			expected: "{}",
			isNil:    false,
		},
		{
			name:     "Nil Outputs",
			outputs:  cty.NilVal,
			expected: "",
			isNil:    true,
		},
		{
			name: "Nested Structures",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"parent": cty.ObjectVal(map[string]cty.Value{
					"child": cty.StringVal("value"),
				}),
			}),
			expected: `{"parent":{"child":"value"}}`,
			isNil:    false,
		},
		{
			name: "Different Data Types",
			outputs: cty.ObjectVal(map[string]cty.Value{
				"string": cty.StringVal("text"),
				"number": cty.NumberIntVal(42),
				"bool":   cty.BoolVal(true),
				"list":   cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
			expected: `{"string":"text","number":42,"bool":true,"list":["a","b"]}`,
			isNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			err := stack.PrintJSONOutput(&buffer, tt.outputs)
			require.NoError(t, err)
			if tt.isNil {
				assert.Equal(t, tt.expected, buffer.String())
			} else {
				assert.JSONEq(t, tt.expected, buffer.String())
			}
		})
	}
}
