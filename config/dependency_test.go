package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/gocty"
)

func TestDecodeDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	config := `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "sql" {
  config_path = "../sql"
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))

	assert.Equal(t, len(decoded.Dependencies), 2)
	assert.Equal(t, decoded.Dependencies[0].Name, "vpc")
	assert.Equal(t, decoded.Dependencies[0].ConfigPath, "../vpc")
	assert.Equal(t, decoded.Dependencies[1].Name, "sql")
	assert.Equal(t, decoded.Dependencies[1].ConfigPath, "../sql")
}

func TestDecodeNoDependencyBlock(t *testing.T) {
	t.Parallel()

	config := `
locals {
  path = "../vpc"
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))
	assert.Equal(t, len(decoded.Dependencies), 0)
}

func TestDecodeDependencyNoLabelIsError(t *testing.T) {
	t.Parallel()

	config := `
dependency {
  config_path = "../vpc"
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.Error(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))
}

func TestDecodeDependencyMockOutputs(t *testing.T) {
	t.Parallel()

	config := `
dependency "hitchhiker" {
  config_path = "../answers"
  mock_outputs = {
    the_answer = 42
  }
  mock_outputs_allowed_terraform_commands = ["validate", "apply"]
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))

	assert.Equal(t, len(decoded.Dependencies), 1)
	dependency := decoded.Dependencies[0]
	assert.Equal(t, dependency.Name, "hitchhiker")
	assert.Equal(t, dependency.ConfigPath, "../answers")

	ctyValueDefault := dependency.MockOutputs
	require.NotNil(t, ctyValueDefault)

	var actualDefault struct {
		TheAnswer int `cty:"the_answer"`
	}
	require.NoError(t, gocty.FromCtyValue(*ctyValueDefault, &actualDefault))
	assert.Equal(t, actualDefault.TheAnswer, 42)

	defaultAllowedCommands := dependency.MockOutputsAllowedTerraformCommands
	require.NotNil(t, defaultAllowedCommands)
	assert.Equal(t, *defaultAllowedCommands, []string{"validate", "apply"})
}
