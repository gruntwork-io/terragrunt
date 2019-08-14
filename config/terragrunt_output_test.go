package config

import (
	"testing"

	"github.com/hashicorp/hcl2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeTerragruntOutputBlockMultiple(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../vpc"
}

terragrunt_output "sql" {
  config_path = "../sql"
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntOutput{}
	require.NoError(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))

	assert.Equal(t, len(decoded.TerragruntOutput), 2)
	assert.Equal(t, decoded.TerragruntOutput[0].Name, "vpc")
	assert.Equal(t, decoded.TerragruntOutput[0].ConfigPath, "../vpc")
	assert.Equal(t, decoded.TerragruntOutput[1].Name, "sql")
	assert.Equal(t, decoded.TerragruntOutput[1].ConfigPath, "../sql")
}

func TestDecodeNoTerragruntOutputBlock(t *testing.T) {
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

	decoded := terragruntOutput{}
	require.NoError(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))
	assert.Equal(t, len(decoded.TerragruntOutput), 0)
}

func TestDecodeTerragruntOutputNoLabelIsError(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output {
  config_path = "../vpc"
}
`
	filename := DefaultTerragruntConfigPath
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, config, filename)
	require.NoError(t, err)

	decoded := terragruntOutput{}
	require.Error(t, decodeHcl(file, filename, &decoded, mockOptionsForTest(t), EvalContextExtensions{}))
}
