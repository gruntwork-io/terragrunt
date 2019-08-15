package config

import (
	"testing"

	"github.com/hashicorp/hcl2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
