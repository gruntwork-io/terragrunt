package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureGraphDAG = "fixtures/dag-graph"

func TestDagGraphFlagsRegistration(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt dag graph -h")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "--queue-ignore-dag-order", "queue-ignore-dag-order flag should be present")
	assert.Contains(t, stdout, "--queue-ignore-errors", "queue-ignore-errors flag should be present")
}

func TestIncludeExternalInDagGraphCmd(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraphDAG)
	workDir := filepath.Join(tmpEnvPath, testFixtureGraphDAG, "region-1")
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
	workDir := filepath.Join(tmpEnvPath, testFixtureGraphDAG, "region-1")
	workDir, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)

	cmd := "terragrunt list --format=dot --dependencies --working-dir " + workDir

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout, "unit-a\" ->")
}
