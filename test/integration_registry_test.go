package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	registryFixturePath                          = "fixture-tfr"
	registryFixtureRootModulePath                = "root"
	registryFixtureRootShorthandModulePath       = "root-shorthand"
	registryFixtureSubdirModulePath              = "subdir"
	registryFixtureSubdirWithReferenceModulePath = "subdir-with-reference"
)

func TestTerraformRegistryFetchingRootModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureRootModulePath, "root_null_resource")
}

func TestTerraformRegistryFetchingRootShorthandModule(t *testing.T) {
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
	modFullPath := util.JoinPath(registryFixturePath, modPath)
	cleanupTerraformFolder(t, modFullPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", modFullPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", modFullPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	_, hasOutput := outputs[expectedOutputKey]
	assert.True(t, hasOutput)

}
