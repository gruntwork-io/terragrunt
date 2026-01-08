package config_test

import (
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
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
	ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), mockOptionsForTestWithConfigPath(t, filename))
	opts, err := options.NewTerragruntOptionsForTest(filename)
	require.NoError(t, err)

	pctx.TerragruntOptions = opts
	err = pctx.TerragruntOptions.Experiments.EnableExperiment(experiment.DependencyFetchOutputFromState)
	require.NoError(t, err)

	pctx.TerragruntOptions.Env = env.Parse(os.Environ())
	tfConfig, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), filename, nil)
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

// TestDisabledDependencyWithNullConfigPath verifies that disabled dependencies
// with null config_path don't panic during parsing (they bypass validation).
func TestDisabledDependencyWithNullConfigPath(t *testing.T) {
	t.Parallel()

	// This config has a disabled dependency with config_path that would fail
	// validation if it were enabled (uses a local that resolves to null)
	cfg := `
locals {
  disabled_path = null
}

dependency "disabled" {
  config_path = local.disabled_path
  enabled     = false
}

dependency "enabled" {
  config_path = "../vpc"
}
`
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTestWithConfigPath(t, config.DefaultTerragruntConfigPath))
	pctx = pctx.WithDecodeList(config.DependencyBlock)

	// Should not panic - disabled deps bypass config_path validation
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	// Only enabled dependency should be in the paths
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
}

// TestDisabledDependencyWithEmptyConfigPath verifies that disabled dependencies
// with empty config_path don't cause errors.
func TestDisabledDependencyWithEmptyConfigPath(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "disabled" {
  config_path = ""
  enabled     = false
}

dependency "enabled" {
  config_path = "../vpc"
}
`
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTestWithConfigPath(t, config.DefaultTerragruntConfigPath))
	pctx = pctx.WithDecodeList(config.DependencyBlock)

	// Should not error - disabled deps bypass config_path validation
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	// Only enabled dependency should be in the paths
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
}
