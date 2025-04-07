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
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "value1", buffer.String(), "String values should be printed without quotes")
}

func TestPrintRawOutputsNumber(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.NumberIntVal(42),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "42", buffer.String(), "Number values should be printed as is")
}

func TestPrintRawOutputsBoolean(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.BoolVal(true),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "true", buffer.String(), "Boolean values should be printed as is")
}

func TestPrintRawOutputsComplexObject(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.ObjectVal(map[string]cty.Value{
			"nested": cty.StringVal("value"),
		}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.Error(t, err, "Complex objects should return an error")
	assert.Contains(t, err.Error(), "Unsupported value for raw output")
	assert.Contains(t, err.Error(), "key1")
	assert.Contains(t, err.Error(), "object")
}

func TestPrintRawOutputsMultipleKeys(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.StringVal("value1"),
		"key2": cty.NumberIntVal(2),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.Error(t, err, "Multiple keys should return an error")
	assert.Contains(t, err.Error(), "requires a single output value")
}

func TestPrintRawOutputsList(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	outputs := cty.ObjectVal(map[string]cty.Value{
		"key1": cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.Error(t, err, "List values should return an error")
	assert.Contains(t, err.Error(), "Unsupported value for raw output")
	assert.Contains(t, err.Error(), "key1")
	assert.Contains(t, err.Error(), "list")
}

func TestPrintRawOutputsNil(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := stack.PrintRawOutputs(nil, &buffer, cty.NilVal)
	require.NoError(t, err)
	assert.Empty(t, buffer.String())
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

func TestPrintRawOutputsNestedValue(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	// Create a nested object structure similar to what FilterOutputs would produce
	// for a path like "parent.child.value"
	outputs := cty.ObjectVal(map[string]cty.Value{
		"parent": cty.ObjectVal(map[string]cty.Value{
			"child": cty.ObjectVal(map[string]cty.Value{
				"value": cty.StringVal("nested_text"),
			}),
		}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "nested_text", buffer.String(), "Should extract the nested value")
}

func TestPrintRawOutputsNestedValueNumber(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	// Create a nested object structure with a number at the leaf
	outputs := cty.ObjectVal(map[string]cty.Value{
		"parent": cty.ObjectVal(map[string]cty.Value{
			"child": cty.ObjectVal(map[string]cty.Value{
				"value": cty.NumberIntVal(42),
			}),
		}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "42", buffer.String(), "Should extract the nested number value")
}

func TestPrintRawOutputsNestedValueBool(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	// Create a nested object structure with a boolean at the leaf
	outputs := cty.ObjectVal(map[string]cty.Value{
		"parent": cty.ObjectVal(map[string]cty.Value{
			"child": cty.ObjectVal(map[string]cty.Value{
				"value": cty.BoolVal(true),
			}),
		}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.NoError(t, err)
	assert.Equal(t, "true", buffer.String(), "Should extract the nested boolean value")
}

func TestPrintRawOutputsNestedMultipleKeys(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	// Create a nested object with multiple keys at a level, which should error
	outputs := cty.ObjectVal(map[string]cty.Value{
		"parent": cty.ObjectVal(map[string]cty.Value{
			"child1": cty.StringVal("value1"),
			"child2": cty.StringVal("value2"),
		}),
	})

	err := stack.PrintRawOutputs(nil, &buffer, outputs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unsupported value for raw output")
}
