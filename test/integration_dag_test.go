package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureGraphDAG = "fixtures/dag-graph"

func TestIncludeExternalInDagGraphCmd(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraphDAG)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGraphDAG)
	helpers.CleanupTerraformFolder(t, rootPath)
	workDir := filepath.Join(rootPath, "region-1")
	workDir, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)

	cmd := "terragrunt dag graph --working-dir " + workDir

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout, "unit-a\" ->")
}

func TestIncludeExternalInDagGraphCmdWithList(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraphDAG)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGraphDAG)
	helpers.CleanupTerraformFolder(t, rootPath)
	workDir := filepath.Join(rootPath, "region-1")
	workDir, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)

	cmd := "terragrunt list --format=dot --dependencies --working-dir " + workDir

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout, "unit-a\" ->")
}
