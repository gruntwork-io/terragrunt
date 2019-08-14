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

	assert.False(t, terragruntConfig.Skip)
	assert.False(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
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
	assert.True(t, terragruntConfig.PreventDestroy)

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
	assert.False(t, terragruntConfig.PreventDestroy)
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
	assert.True(t, terragruntConfig.PreventDestroy)
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
	assert.False(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseTerragruntOutputBlockSetsTerragruntOutputs(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntOutputBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.TerragruntOutputs)
	assert.Equal(t, len(terragruntConfig.TerragruntOutputs), 1)
	assert.Equal(t, terragruntConfig.TerragruntOutputs[0].Name, "vpc")
	assert.Equal(t, terragruntConfig.TerragruntOutputs[0].ConfigPath, "../app1")
}

func TestPartialParseMultipleTerragruntOutputBlockSetsTerragruntOutputs(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}

terragrunt_output "sql" {
  config_path = "../db1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntOutputBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.TerragruntOutputs)
	assert.Equal(t, len(terragruntConfig.TerragruntOutputs), 2)
	assert.Equal(t, terragruntConfig.TerragruntOutputs[0].Name, "vpc")
	assert.Equal(t, terragruntConfig.TerragruntOutputs[0].ConfigPath, "../app1")
	assert.Equal(t, terragruntConfig.TerragruntOutputs[1].Name, "sql")
	assert.Equal(t, terragruntConfig.TerragruntOutputs[1].ConfigPath, "../db1")
}

func TestPartialParseTerragruntOutputBlockSetsDependencies(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}

terragrunt_output "sql" {
  config_path = "../db1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntOutputBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 2)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1"})
}

func TestPartialParseTerragruntOutputBlockMergesDependencies(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

terragrunt_output "sql" {
  config_path = "../db1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{DependenciesBlock, TerragruntOutputBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 3)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../vpc", "../app1", "../db1"})
}

func TestPartialParseTerragruntOutputBlockMergesDependenciesOrdering(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

terragrunt_output "sql" {
  config_path = "../db1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntOutputBlock, DependenciesBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 3)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1", "../vpc"})
}

func TestPartialParseTerragruntOutputBlockMergesDependenciesDedup(t *testing.T) {
	t.Parallel()

	config := `
terragrunt_output "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../app1"]
}

terragrunt_output "sql" {
  config_path = "../db1"
}
`

	terragruntConfig, err := PartialParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, []PartialDecodeSectionType{TerragruntOutputBlock, DependenciesBlock})
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	require.NotNil(t, terragruntConfig.Dependencies)
	assert.Equal(t, len(terragruntConfig.Dependencies.Paths), 2)
	assert.Equal(t, terragruntConfig.Dependencies.Paths, []string{"../app1", "../db1"})
}
