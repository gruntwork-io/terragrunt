package config

import (
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock})
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

	_, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntFlags})
	assert.NoError(t, err)

	_, err = PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock, TerragruntFlags})
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

	terragruntConfig, err := PartialParseConfigString("", mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock, TerragruntFlags})
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
	_, err := PartialParseConfigFile(opts.TerragruntConfigPath, opts, nil, []PartialDecodeSectionType{TerragruntFlags})
	assert.NoError(t, err)

	_, err = PartialParseConfigFile(opts.TerragruntConfigPath, opts, nil, []PartialDecodeSectionType{DependenciesBlock})
	assert.Error(t, err)
}

func TestPartialParseOnlyInheritsSelectedBlocksFlags(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-partial-parse/partial-inheritance/child/"+DefaultTerragruntConfigPath)
	terragruntConfig, err := PartialParseConfigFile(opts.TerragruntConfigPath, opts, nil, []PartialDecodeSectionType{TerragruntFlags})
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
	terragruntConfig, err := PartialParseConfigFile(opts.TerragruntConfigPath, opts, nil, []PartialDecodeSectionType{DependenciesBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependencyBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependencyBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependencyBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock, DependencyBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependencyBlock, DependenciesBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependencyBlock, DependenciesBlock})
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

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerraformSource})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, *terragruntConfig.Terraform.Source, "../../modules/app")
}
