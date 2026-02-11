package util_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decodeStringBoolConfig struct {
	PointerValue *bool  `mapstructure:"pointer_value"`
	StringValue  string `mapstructure:"string_value"`
	BoolValue    bool   `mapstructure:"bool_value"`
}

func TestDecodeWithStringBoolHook(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"bool_value":    "true",
		"pointer_value": "false",
		"string_value":  "plain-string",
	}

	var output decodeStringBoolConfig

	err := util.DecodeWithStringBoolHook(input, &output)
	require.NoError(t, err)

	assert.True(t, output.BoolValue)
	require.NotNil(t, output.PointerValue)
	assert.False(t, *output.PointerValue)
	assert.Equal(t, "plain-string", output.StringValue)
}

func TestDecodeWithStringBoolHook_NativeBoolPassthrough(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"bool_value":   true,
		"string_value": "hello",
	}

	var output decodeStringBoolConfig

	err := util.DecodeWithStringBoolHook(input, &output)
	require.NoError(t, err)

	assert.True(t, output.BoolValue)
	assert.Equal(t, "hello", output.StringValue)
}

func TestDecodeWithStringBoolHook_CaseInsensitiveAndWhitespace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"uppercase-TRUE", "TRUE", true},
		{"mixed-True", "True", true},
		{"uppercase-FALSE", "FALSE", false},
		{"mixed-False", "False", false},
		{"whitespace-true", "  true  ", true},
		{"whitespace-false", " false ", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := map[string]any{"bool_value": tc.input}

			var output decodeStringBoolConfig

			err := util.DecodeWithStringBoolHook(input, &output)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, output.BoolValue)
		})
	}
}

func TestDecodeWithStringBoolHook_InvalidBoolStringsRejected(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input any
		name  string
	}{
		{
			name: "one-string",
			input: map[string]any{
				"bool_value": "1",
			},
		},
		{
			name: "empty-string",
			input: map[string]any{
				"bool_value": "",
			},
		},
		{
			name: "arbitrary-string",
			input: map[string]any{
				"bool_value": "maybe",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var output decodeStringBoolConfig

			err := util.DecodeWithStringBoolHook(tc.input, &output)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid boolean string")
		})
	}
}

func TestDecodeWithStringBoolHook_NonStringBoolsRejected(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"bool_value": 1,
	}

	var output decodeStringBoolConfig

	err := util.DecodeWithStringBoolHook(input, &output)
	require.Error(t, err)
}
