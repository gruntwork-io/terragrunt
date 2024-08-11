package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanVariables(t *testing.T) {
	t.Parallel()

	opts := terragruntOptionsForTest(t, "")

	inputs, err := ParseVariables(opts, "../test/fixture-inputs")
	require.NoError(t, err)
	require.Len(t, inputs, 11)

	varByName := map[string]*ParsedVariable{}
	for _, input := range inputs {
		varByName[input.Name] = input
	}

	require.Equal(t, "string", varByName["string"].Type)
	require.Equal(t, "\"\"", varByName["string"].DefaultValuePlaceholder)

	require.Equal(t, "bool", varByName["bool"].Type)
	require.Equal(t, "false", varByName["bool"].DefaultValuePlaceholder)

	require.Equal(t, "number", varByName["number"].Type)
	require.Equal(t, "0", varByName["number"].DefaultValuePlaceholder)

	require.Equal(t, "object", varByName["object"].Type)
	require.Equal(t, "{}", varByName["object"].DefaultValuePlaceholder)

	require.Equal(t, "map", varByName["map_bool"].Type)
	require.Equal(t, "{}", varByName["map_bool"].DefaultValuePlaceholder)

	require.Equal(t, "list", varByName["list_bool"].Type)
	require.Equal(t, "[]", varByName["list_bool"].DefaultValuePlaceholder)
}

func TestScanDefaultVariables(t *testing.T) {
	t.Parallel()
	opts := terragruntOptionsForTest(t, "")

	inputs, err := ParseVariables(opts, "../test/fixture-inputs-defaults")
	require.NoError(t, err)
	require.Len(t, inputs, 11)

	varByName := map[string]*ParsedVariable{}
	for _, input := range inputs {
		varByName[input.Name] = input
	}

	require.Equal(t, "string", varByName["project_name"].Type)
	require.Equal(t, "Project name", varByName["project_name"].Description)
	require.Equal(t, "\"\"", varByName["project_name"].DefaultValuePlaceholder)

	require.Equal(t, "(variable no_type_value_var does not define a type)", varByName["no_type_value_var"].Type)
	require.Equal(t, "(variable no_type_value_var did not define a description)", varByName["no_type_value_var"].Description)
	require.Equal(t, "\"\"", varByName["no_type_value_var"].DefaultValuePlaceholder)

	require.Equal(t, "number", varByName["number_default"].Type)
	require.Equal(t, "number variable with default", varByName["number_default"].Description)
	require.Equal(t, "42", varByName["number_default"].DefaultValue)
	require.Equal(t, "0", varByName["number_default"].DefaultValuePlaceholder)

	require.Equal(t, "object", varByName["object_var"].Type)
	require.Equal(t, "{\"num\":42,\"str\":\"default\"}", varByName["object_var"].DefaultValue)

	require.Equal(t, "map", varByName["map_var"].Type)
	require.Equal(t, "{\"key\":\"value42\"}", varByName["map_var"].DefaultValue)

	require.Equal(t, "bool", varByName["enabled"].Type)
	require.Equal(t, "true", varByName["enabled"].DefaultValue)
	require.Equal(t, "Enable or disable the module", varByName["enabled"].Description)

	require.Equal(t, "string", varByName["vpc"].Type)
	require.Equal(t, "\"default-vpc\"", varByName["vpc"].DefaultValue)
	require.Equal(t, "VPC to be used", varByName["vpc"].Description)
}
