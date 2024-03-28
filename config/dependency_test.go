package config

import (
	"context"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/hashicorp/hcl/v2"
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
	file, err := hclparse.NewParser().ParseFromString(config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

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
	file, err := hclparse.NewParser().ParseFromString(config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
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
	file, err := hclparse.NewParser().ParseFromString(config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.Error(t, file.Decode(&decoded, &hcl.EvalContext{}))
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
	file, err := hclparse.NewParser().ParseFromString(config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

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
func TestParseDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	filename := "../test/fixture-regressions/multiple-dependency-load-sync/main/terragrunt.hcl"
	ctx := NewParsingContext(context.Background(), mockOptionsForTestWithConfigPath(t, filename))
	ctx.TerragruntOptions.FetchDependencyOutputFromState = true
	ctx.TerragruntOptions.Env = env.Parse(os.Environ())
	opts, err := options.NewTerragruntOptionsForTest(filename)
	require.NoError(t, err)
	tfConfig, err := ParseConfigFile(opts, ctx, filename, nil)
	require.NoError(t, err)
	require.Len(t, tfConfig.TerragruntDependencies, 2)
	assert.Equal(t, tfConfig.TerragruntDependencies[0].Name, "dependency_1")
	assert.Equal(t, tfConfig.TerragruntDependencies[1].Name, "dependency_2")
}

func TestDisabledDependency(t *testing.T) {
	t.Parallel()

	config := `
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
dependency "vpc" {
  config_path = "../vpc"
}
`
	filename := DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(config, filename)
	require.NoError(t, err)

	decoded := terragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
	assert.Equal(t, len(decoded.Dependencies), 2)
}
