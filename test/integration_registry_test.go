package test_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	registryFixturePath                          = "fixtures/tfr"
	registryFixtureRootModulePath                = "root"
	registryFixtureRootShorthandModulePath       = "root-shorthand"
	registryFixtureSubdirModulePath              = "subdir"
	registryFixtureSubdirWithReferenceModulePath = "subdir-with-reference"
	registryFixtureVersion                       = "version"
)

func TestTerraformRegistryFetchingRootModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureRootModulePath, "root_null_resource")
}

func TestRegistryFetchingRootShorthandModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureRootShorthandModulePath, "root_null_resource")
}

func TestTerraformRegistryFetchingSubdirModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureSubdirModulePath, "one_null_resource")
}

func TestTerraformRegistryFetchingSubdirWithReferenceModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureSubdirWithReferenceModulePath, "two")
}

func testTerraformRegistryFetching(t *testing.T, modPath, expectedOutputKey string) {
	t.Helper()

	modFullPath := util.JoinPath(registryFixturePath, modPath)
	helpers.CleanupTerraformFolder(t, modFullPath)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+modFullPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+modFullPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	_, hasOutput := outputs[expectedOutputKey]
	assert.True(t, hasOutput)
}

// test that the version of the module is correctly resolved and downloaded when running a terragrunt init
func TestTerraformRegistryVersionResolution(t *testing.T) {
	t.Parallel()

	versionFixture := util.JoinPath(registryFixturePath, registryFixtureVersion)
	helpers.CleanupTerragruntFolder(t, versionFixture)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --working-dir "+versionFixture)
	require.NoError(t, err)
}
