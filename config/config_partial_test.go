package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPartialParseResolvesLocals(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
  app1 = "../app1"
}

dependencies {
  paths = [local.app1]
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])
	assert.Equal(t, map[string]any{"app1": "../app1"}, terragruntConfig.Locals)

	assert.Nil(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
}

func TestPartialParseDoesNotResolveIgnoredBlock(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  # This function call will fail when attempting to decode
  paths = [file("i-am-a-file-that-does-not-exist")]
}

prevent_destroy = false
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	_, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	_, err = config.PartialParseConfigString(ctx.WithDecodeList(config.DependenciesBlock), l, config.DefaultTerragruntConfigPath, cfg, nil)
	assert.Error(t, err)
}

func TestPartialParseMultipleItems(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  paths = ["../app1"]
}

prevent_destroy = true
skip = true
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])

	assert.NotNil(t, terragruntConfig.Skip)
	assert.True(t, *terragruntConfig.Skip)
	assert.True(t, *terragruntConfig.PreventDestroy)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOmittedItems(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, "", nil)

	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDoesNotResolveIgnoredBlockEvenInParent(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/partial-parse/ignore-bad-block-in-parent/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)
	_, err := config.PartialParseConfigFile(ctx.WithDecodeList(config.TerragruntFlags), l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	_, err = config.PartialParseConfigFile(ctx.WithDecodeList(config.DependenciesBlock), l, opts.TerragruntConfigPath, nil)
	assert.Error(t, err)
}

func TestPartialParseOnlyInheritsSelectedBlocksFlags(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/partial-parse/partial-inheritance/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts).WithDecodeList(config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigFile(ctx, l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.Skip)
	assert.True(t, *terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOnlyInheritsSelectedBlocksDependencies(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/partial-parse/partial-inheritance/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts).WithDecodeList(config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigFile(ctx, l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])

	assert.Nil(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 1)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
}

func TestPartialParseMultipleDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 2)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
	assert.Equal(t, "sql", terragruntConfig.TerragruntDependencies[1].Name)
	assert.Equal(t, cty.StringVal("../db1"), terragruntConfig.TerragruntDependencies[1].ConfigPath)
}

func TestPartialParseDependencyBlockSetsDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 2)
	assert.Equal(t, []string{"../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependenciesBlock, config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 3)
	assert.Equal(t, []string{"../vpc", "../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependenciesOrdering(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock, config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 3)
	assert.Equal(t, []string{"../app1", "../db1", "../vpc"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependenciesDedup(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../app1"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock, config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 2)
	assert.Equal(t, []string{"../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseOnlyParsesTerraformSource(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  source = "../../modules/app"
  before_hook "before" {
    commands = ["apply"]
	execute  = ["echo", dependency.vpc.outputs.vpc_id]
  }
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.TerraformSource)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "../../modules/app", *terragruntConfig.Terraform.Source)
}

func TestOptionalDependenciesAreSkipped(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
`

	l := logger.CreateLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t)).WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
}
