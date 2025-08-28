package config_test

import (
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

func TestDecodeDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "sql" {
  config_path = "../sql"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

	assert.Len(t, decoded.Dependencies, 2)
	assert.Equal(t, "vpc", decoded.Dependencies[0].Name)
	assert.Equal(t, cty.StringVal("../vpc"), decoded.Dependencies[0].ConfigPath)
	assert.Equal(t, "sql", decoded.Dependencies[1].Name)
	assert.Equal(t, cty.StringVal("../sql"), decoded.Dependencies[1].ConfigPath)
}

func TestDecodeNoDependencyBlock(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
  path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
	assert.Empty(t, decoded.Dependencies)
}

func TestDecodeDependencyNoLabelIsError(t *testing.T) {
	t.Parallel()

	cfg := `
dependency {
  config_path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.Error(t, file.Decode(&decoded, &hcl.EvalContext{}))
}

func TestDecodeDependencyMockOutputs(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "hitchhiker" {
  config_path = "../answers"
  mock_outputs = {
    the_answer = 42
  }
  mock_outputs_allowed_terraform_commands = ["validate", "apply"]
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

	assert.Len(t, decoded.Dependencies, 1)
	dependency := decoded.Dependencies[0]
	assert.Equal(t, "hitchhiker", dependency.Name)
	assert.Equal(t, cty.StringVal("../answers"), dependency.ConfigPath)

	ctyValueDefault := dependency.MockOutputs
	assert.NotNil(t, ctyValueDefault)

	var actualDefault struct {
		TheAnswer int `cty:"the_answer"`
	}
	require.NoError(t, gocty.FromCtyValue(*ctyValueDefault, &actualDefault))
	assert.Equal(t, 42, actualDefault.TheAnswer)

	defaultAllowedCommands := dependency.MockOutputsAllowedTerraformCommands
	assert.NotNil(t, defaultAllowedCommands)
	assert.Equal(t, []string{"validate", "apply"}, *defaultAllowedCommands)
}
func TestParseDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	filename := "../test/fixtures/regressions/multiple-dependency-load-sync/main/terragrunt.hcl"
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), mockOptionsForTestWithConfigPath(t, filename))
	opts, err := options.NewTerragruntOptionsForTest(filename)
	require.NoError(t, err)
	ctx.TerragruntOptions = opts
	ctx.TerragruntOptions.FetchDependencyOutputFromState = true
	ctx.TerragruntOptions.Env = env.Parse(os.Environ())
	tfConfig, err := config.ParseConfigFile(ctx, logger.CreateLogger(), filename, nil)
	require.NoError(t, err)
	assert.Len(t, tfConfig.TerragruntDependencies, 2)
	assert.Equal(t, "dependency_1", tfConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, "dependency_2", tfConfig.TerragruntDependencies[1].Name)
}

func TestDisabledDependency(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
dependency "vpc" {
  config_path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
	assert.Len(t, decoded.Dependencies, 2)
}

func TestValidateUniqueConfigPaths_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hclCode string
	}{
		{
			name: "unique config paths",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "database" {
  config_path = "../database"
}
`,
		},
		{
			name: "single dependency",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}
`,
		},
		{
			name: "no dependencies",
			hclCode: `
locals {
  vpc_id = "vpc-123"
}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			filename := config.DefaultTerragruntConfigPath
			file, err := hclparse.NewParser().ParseFromString(test.hclCode, filename)
			require.NoError(t, err)

			decoded := config.TerragruntDependency{}
			err = file.Decode(&decoded, &hcl.EvalContext{})
			require.NoError(t, err)

			// Also test the validation function directly
			err = config.ValidateUniqueConfigPaths(decoded.Dependencies)
			require.NoError(t, err)
		})
	}
}

func TestValidateUniqueConfigPaths_Failure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		hclCode     string
		expectedErr string
	}{
		{
			name: "duplicate config paths",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "network" {
  config_path = "../vpc"
}
`,
			expectedErr: "duplicate config_path '../vpc' found in dependency blocks. Dependency 'vpc' and dependency 'network' both point to the same config path",
		},
		{
			name: "multiple duplicates",
			hclCode: `
dependency "app1" {
  config_path = "../app"
}

dependency "app2" {
  config_path = "../app"
}

dependency "app3" {
  config_path = "../app"
}
`,
			expectedErr: "duplicate config_path '../app' found in dependency blocks. Dependency 'app1' and dependency 'app2' both point to the same config path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			filename := config.DefaultTerragruntConfigPath
			file, err := hclparse.NewParser().ParseFromString(test.hclCode, filename)
			require.NoError(t, err)

			decoded := config.TerragruntDependency{}
			err = file.Decode(&decoded, &hcl.EvalContext{})
			require.NoError(t, err)

			// Test the validation function directly
			err = config.ValidateUniqueConfigPaths(decoded.Dependencies)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}
