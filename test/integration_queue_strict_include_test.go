package test_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureQueueStrictInclude = "fixtures/queue-strict-include"
)

// TestQueueStrictIncludeWithDependencyNotInQueue tests that when using --queue-strict-include
// or --filter, units can run even when their dependencies are not in the queue (but have existing state).
func TestQueueStrictIncludeWithDependencyNotInQueue(t *testing.T) {
	t.Parallel()

	// Create test fixture with dependency chain: transitive-dependency -> dependency -> dependent
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureQueueStrictInclude)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureQueueStrictInclude)

	// First, apply all units to create state
	// This simulates the scenario where units have been previously applied
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		fmt.Sprintf("terragrunt run --log-level debug --all --non-interactive --working-dir %s -- apply -auto-approve", testPath))
	require.NoError(t, err, "Failed to apply all units initially\nstdout: %s\nstderr: %s", stdout, stderr)

	// Verify all units were applied
	assert.Contains(t, stdout+stderr, "transitive-dependency", "transitive-dependency should be applied")
	assert.Contains(t, stdout+stderr, "dependency", "dependency should be applied")
	assert.Contains(t, stdout+stderr, "dependent", "dependent should be applied")

	t.Run("queue-strict-include with queue-include-dir", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testPath)

		// Test with --queue-strict-include and --queue-include-dir to only include dependency
		// The dependency unit depends on transitive-dependency, which should not be in the queue
		// but should be considered ready because it has existing state
		cmd := fmt.Sprintf("terragrunt run --log-level debug --all --non-interactive --working-dir %s --queue-include-dir '**/dependency' --queue-strict-include -- plan", testPath)
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// The command should succeed
		require.NoError(t, err, "Command should succeed when dependency has existing state\nstdout: %s\nstderr: %s", stdout, stderr)

		// Verify that dependency unit was processed
		output := stdout + stderr
		assert.Contains(t, output, "dependency", "dependency unit should be processed")

		// Verify that transitive-dependency is NOT in the queue (filtered out)
		// but dependency still runs successfully
		assert.Contains(t, output, "found 1 readyEntries tasks",
			"Should show 'found 1 readyEntries tasks' - dependency should run even though transitive-dependency is not in queue")
	})

	t.Run("queue-strict-include with queue-include-dir and destroy", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testPath)

		// Test with --queue-strict-include and --queue-include-dir to only include dependency
		// The dependency unit depends on transitive-dependency, which should not be in the queue
		// but should be considered ready because it has existing state
		cmd := fmt.Sprintf("terragrunt run --log-level debug --all --non-interactive --working-dir %s --queue-include-dir '**/dependency' --queue-strict-include -- destroy -auto-approve", testPath)
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// The command should succeed
		require.NoError(t, err, "Command should succeed when dependency has existing state\nstdout: %s\nstderr: %s", stdout, stderr)

		// Verify that dependency unit was processed
		output := stdout + stderr
		assert.Contains(t, output, "dependency", "dependency unit should be processed")

		// Verify that transitive-dependency is NOT in the queue (filtered out)
		// but dependency still runs successfully
		assert.Contains(t, output, "found 1 readyEntries tasks",
			"Should show 'found 1 readyEntries tasks' - dependency should run even though transitive-dependency is not in queue")
	})

	t.Run("experimental filter flag", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testPath)

		// Skip if filter-flag experiment is not enabled
		if !helpers.IsExperimentMode(t) {
			t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
		}

		// Test with experimental --filter to only include dependency
		cmd := fmt.Sprintf("terragrunt run --log-level debug --all --non-interactive --experiment-mode --working-dir %s --filter './dependency' -- plan", testPath)
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// The command should succeed
		require.NoError(t, err, "Command should succeed when dependency has existing state\nstdout: %s\nstderr: %s", stdout, stderr)

		// Verify that dependency unit was processed
		output := stdout + stderr
		assert.Contains(t, output, "dependency", "dependency unit should be processed")

		// Verify that transitive-dependency is NOT in the queue (filtered out)
		// but dependency still runs successfully
		assert.Contains(t, output, "found 1 readyEntries tasks",
			"Should show 'found 1 readyEntries tasks' - dependency should run even though transitive-dependency is not in queue")
	})

	t.Run("experimental filter flag and destroy", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testPath)

		// Skip if filter-flag experiment is not enabled
		if !helpers.IsExperimentMode(t) {
			t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
		}

		// Test with experimental --filter to only include dependency
		cmd := fmt.Sprintf("terragrunt run --log-level debug --all --non-interactive --experiment-mode --working-dir %s --filter './dependency' -- destroy -auto-approve", testPath)
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// The command should succeed
		require.NoError(t, err, "Command should succeed when dependency has existing state\nstdout: %s\nstderr: %s", stdout, stderr)

		// Verify that dependency unit was processed
		output := stdout + stderr
		assert.Contains(t, output, "dependency", "dependency unit should be processed")

		// Verify that transitive-dependency is NOT in the queue (filtered out)
		// but dependency still runs successfully
		assert.Contains(t, output, "found 1 readyEntries tasks",
			"Should show 'found 1 readyEntries tasks' - dependency should run even though transitive-dependency is not in queue")
	})
}
