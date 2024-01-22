package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartialParseResolvesLocals(t *testing.T) {
	t.Parallel()

	config := `
locals {
  app1 = "../app1"
}

dependencies {
  paths = [local.app1]
}
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependenciesBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 1)
	assert.Equal(t, terragruntConfig.Dependencies.Paths[0], "../app1")
	assert.Equal(t, map[string]interface{}{"app1": "../app1"}, terragruntConfig.Locals)

	assert.False(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
}

func TestPartialParseDoesNotResolveIgnoredBlock(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
  # This function call will fail when attempting to decode
  paths = [file("i-am-a-file-that-does-not-exist")]
}

prevent_destroy = false
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t))
	_, err := PartialParseConfigString(ctx.WithDecodeList(TerragruntFlags), DefaultTerragruntConfigPath, config, nil)
	assert.NoError(t, err)

	_, err = PartialParseConfigString(ctx.WithDecodeList(DependenciesBlock), DefaultTerragruntConfigPath, config, nil)
	assert.Error(t, err)
}

func TestPartialParseMultipleItems(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
  paths = ["../app1"]
}

prevent_destroy = true
skip = true
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependenciesBlock, TerragruntFlags)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 1)
	assert.Equal(t, terragruntConfig.Dependencies.Paths[0], "../app1")

	assert.True(t, terragruntConfig.Skip)
	assert.True(t, *terragruntConfig.PreventDestroy)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOmittedItems(t *testing.T) {
	t.Parallel()

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependenciesBlock, TerragruntFlags)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, "", nil)

	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.False(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDoesNotResolveIgnoredBlockEvenInParent(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-partial-parse/ignore-bad-block-in-parent/child/"+DefaultTerragruntConfigPath)

	ctx := NewParsingContext(context.Background(), opts)
	_, err := PartialParseConfigFile(ctx.WithDecodeList(TerragruntFlags), opts.TerragruntConfigPath, nil)
	assert.NoError(t, err)

	_, err = PartialParseConfigFile(ctx.WithDecodeList(DependenciesBlock), opts.TerragruntConfigPath, nil)
	assert.Error(t, err)
}

func TestPartialParseOnlyInheritsSelectedBlocksFlags(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-partial-parse/partial-inheritance/child/"+DefaultTerragruntConfigPath)

	ctx := NewParsingContext(context.Background(), opts).WithDecodeList(TerragruntFlags)
	terragruntConfig, err := PartialParseConfigFile(ctx, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.False(t, terragruntConfig.Skip)
	assert.True(t, *terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOnlyInheritsSelectedBlocksDependencies(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-partial-parse/partial-inheritance/child/"+DefaultTerragruntConfigPath)

	ctx := NewParsingContext(context.Background(), opts).WithDecodeList(DependenciesBlock)
	terragruntConfig, err := PartialParseConfigFile(ctx, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 1)
	assert.Equal(t, terragruntConfig.Dependencies.Paths[0], "../app1")

	assert.False(t, terragruntConfig.Skip)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	config := `
dependency "vpc" {
  config_path = "../app1"
}
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Equal(t, len(terragruntConfig.TerragruntDependencies), 1)
	assert.Equal(t, terragruntConfig.TerragruntDependencies[0].Name, "vpc")
	assert.Equal(t, terragruntConfig.TerragruntDependencies[0].ConfigPath, "../app1")
}

func TestPartialParseMultipleDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	config := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Equal(t, len(terragruntConfig.TerragruntDependencies), 2)
	assert.Equal(t, terragruntConfig.TerragruntDependencies[0].Name, "vpc")
	assert.Equal(t, terragruntConfig.TerragruntDependencies[0].ConfigPath, "../app1")
	assert.Equal(t, terragruntConfig.TerragruntDependencies[1].Name, "sql")
	assert.Equal(t, terragruntConfig.TerragruntDependencies[1].ConfigPath, "../db1")
}

func TestPartialParseDependencyBlockSetsDependencies(t *testing.T) {
	t.Parallel()

	config := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 2)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1"})
}

func TestPartialParseDependencyBlockMergesDependencies(t *testing.T) {
	t.Parallel()

	config := `
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

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependenciesBlock, DependencyBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 3)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../vpc", "../app1", "../db1"})
}

func TestPartialParseDependencyBlockMergesDependenciesOrdering(t *testing.T) {
	t.Parallel()

	config := `
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

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock, DependenciesBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 3)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1", "../vpc"})
}

func TestPartialParseDependencyBlockMergesDependenciesDedup(t *testing.T) {
	t.Parallel()

	config := `
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

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock, DependenciesBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 2)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1"})
}

func TestPartialParseOnlyParsesTerraformSource(t *testing.T) {
	t.Parallel()

	config := `
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

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(TerraformSource)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, *terragruntConfig.Terraform.Source, "../../modules/app")
}

func TestOptionalDependenciesAreSkipped(t *testing.T) {
	t.Parallel()

	config := `
dependency "vpc" {
  config_path = "../vpc"
}
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
`

	ctx := NewParsingContext(context.Background(), mockOptionsForTest(t)).WithDecodeList(DependencyBlock)
	terragruntConfig, err := PartialParseConfigString(ctx, DefaultTerragruntConfigPath, config, nil)
	require.NoError(t, err)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
}
