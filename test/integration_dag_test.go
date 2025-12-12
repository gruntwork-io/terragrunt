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

	assert.Contains(t, stdout, "--queue-exclude-dir", "queue-exclude-dir flag should be present")
	assert.Contains(t, stdout, "--queue-excludes-file", "queue-excludes-file flag should be present")
	assert.Contains(t, stdout, "--queue-ignore-dag-order", "queue-ignore-dag-order flag should be present")
	assert.Contains(t, stdout, "--queue-ignore-errors", "queue-ignore-errors flag should be present")
	assert.Contains(t, stdout, "--queue-include-dir", "queue-include-dir flag should be present")
}

func TestIncludeExternalInDagGraphCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGraphDAG)
	workDir := filepath.Join(testFixtureGraphDAG, "region-1")
	workDir, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)

	cmd := "terragrunt dag graph --queue-include-external --working-dir " + workDir

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout, "unit-a\" ->")
}
