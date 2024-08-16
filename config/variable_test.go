package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanVariables(t *testing.T) {
	t.Parallel()

	opts := terragruntOptionsForTest(t, "")

	inputs, err := config.ParseVariables(opts, "../test/fixture-inputs")
	require.NoError(t, err)
	assert.Len(t, inputs, 11)

	varByName := map[string]*config.ParsedVariable{}
	for _, input := range inputs {
		varByName[input.Name] = input
	}

	assert.Equal(t, "string", varByName["string"].Type)
	assert.Equal(t, "\"\"", varByName["string"].DefaultValuePlaceholder)

	assert.Equal(t, "bool", varByName["bool"].Type)
	assert.Equal(t, "false", varByName["bool"].DefaultValuePlaceholder)

	assert.Equal(t, "number", varByName["number"].Type)
	assert.Equal(t, "0", varByName["number"].DefaultValuePlaceholder)

	assert.Equal(t, "object", varByName["object"].Type)
	assert.Equal(t, "{}", varByName["object"].DefaultValuePlaceholder)

	assert.Equal(t, "map", varByName["map_bool"].Type)
	assert.Equal(t, "{}", varByName["map_bool"].DefaultValuePlaceholder)

	assert.Equal(t, "list", varByName["list_bool"].Type)
	assert.Equal(t, "[]", varByName["list_bool"].DefaultValuePlaceholder)
}

func TestScanDefaultVariables(t *testing.T) {
	t.Parallel()
	opts := terragruntOptionsForTest(t, "")

	inputs, err := config.ParseVariables(opts, "../test/fixture-inputs-defaults")
	require.NoError(t, err)
	assert.Len(t, inputs, 11)

	varByName := map[string]*config.ParsedVariable{}
	for _, input := range inputs {
		varByName[input.Name] = input
	}

	assert.Equal(t, "string", varByName["project_name"].Type)
	assert.Equal(t, "Project name", varByName["project_name"].Description)
	assert.Equal(t, "\"\"", varByName["project_name"].DefaultValuePlaceholder)

	assert.Equal(t, "(variable no_type_value_var does not define a type)", varByName["no_type_value_var"].Type)
	assert.Equal(t, "(variable no_type_value_var did not define a description)", varByName["no_type_value_var"].Description)
	assert.Equal(t, "\"\"", varByName["no_type_value_var"].DefaultValuePlaceholder)

	assert.Equal(t, "number", varByName["number_default"].Type)
	assert.Equal(t, "number variable with default", varByName["number_default"].Description)
	assert.Equal(t, "42", varByName["number_default"].DefaultValue)
	assert.Equal(t, "0", varByName["number_default"].DefaultValuePlaceholder)

	assert.Equal(t, "object", varByName["object_var"].Type)
	assert.Equal(t, "{\"num\":42,\"str\":\"default\"}", varByName["object_var"].DefaultValue)

	assert.Equal(t, "map", varByName["map_var"].Type)
	assert.Equal(t, "{\"key\":\"value42\"}", varByName["map_var"].DefaultValue)

	assert.Equal(t, "bool", varByName["enabled"].Type)
	assert.Equal(t, "true", varByName["enabled"].DefaultValue)
	assert.Equal(t, "Enable or disable the module", varByName["enabled"].Description)

	assert.Equal(t, "string", varByName["vpc"].Type)
	assert.Equal(t, "\"default-vpc\"", varByName["vpc"].DefaultValue)
	assert.Equal(t, "VPC to be used", varByName["vpc"].Description)
}
