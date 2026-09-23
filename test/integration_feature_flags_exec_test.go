//go:build exec

// This test hands Terragrunt a shell script as the OpenTofu binary, so it
// spawns a real process and is gated behind the exec tag.

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRunAllIsolated = "fixtures/feature-flags/run-all-isolated-defaults"

// TestExecFeatureFlagRunAllIsolatesPerUnitDefaults verifies run --all keeps feature defaults scoped to each unit.
func TestExecFeatureFlagRunAllIsolatesPerUnitDefaults(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	helpers.CleanupTerraformFolder(t, testRunAllIsolated)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllIsolated)
	rootPath := filepath.Join(tmpEnvPath, testRunAllIsolated)
	livePath := filepath.Join(rootPath, "live")
	targetPath := filepath.Join(livePath, "target-service")
	peerPath := filepath.Join(livePath, "peer-service")
	tfPath := filepath.Join(rootPath, "fake-tf.sh")

	require.NoError(t, os.Chmod(tfPath, 0o755))

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --no-color --inputs-debug --tf-path "+tfPath+" --working-dir "+targetPath,
	)
	require.NoError(t, err)

	targetDirect := readDebugInputs(t, targetPath)
	assert.EqualValues(t, true, targetDirect["effective_toggle"])
	assert.EqualValues(t, true, targetDirect["raw_toggle"])

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, true, true)
	assertDebugInputs(t, peerPath, false, false)

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --feature toggle=true --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, true, true)
	assertDebugInputs(t, peerPath, true, false)

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --feature toggle=false --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, false, true)
	assertDebugInputs(t, peerPath, false, false)
}

// assertDebugInputs verifies the effective and raw toggle values in Terragrunt debug inputs.
func assertDebugInputs(t *testing.T, rootPath string, expectedEffective bool, expectedRaw bool) {
	t.Helper()

	inputs := readDebugInputs(t, rootPath)

	assert.EqualValues(t, expectedEffective, inputs["effective_toggle"])
	assert.EqualValues(t, expectedRaw, inputs["raw_toggle"])
}

// readDebugInputs reads Terragrunt's inputs debug file from the given unit path.
func readDebugInputs(t *testing.T, rootPath string) map[string]any {
	t.Helper()

	debugBytes, err := os.ReadFile(filepath.Join(rootPath, helpers.TerragruntDebugFile))
	require.NoError(t, err)

	inputs := map[string]any{}
	require.NoError(t, json.Unmarshal(debugBytes, &inputs))

	return inputs
}

// removeDebugInputs removes generated Terragrunt inputs debug files from unit paths.
func removeDebugInputs(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		debugPath := filepath.Join(path, helpers.TerragruntDebugFile)
		if err := os.Remove(debugPath); err != nil && !os.IsNotExist(err) {
			require.NoError(t, err)
		}
	}
}
