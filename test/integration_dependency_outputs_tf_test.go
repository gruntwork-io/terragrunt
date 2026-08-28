//go:build tf

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFDependencyOutputSkipDependencyOutputsFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)

	// The subtests all drive the same unit, so each one needs its own copy of the
	// fixture. Sharing a working directory makes them race for its state lock.
	noOutputPath := func(t *testing.T) string {
		t.Helper()

		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)

		return filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration", "skip-dependency-outputs")
	}

	t.Run("plan without flag fails", func(t *testing.T) {
		t.Parallel()
		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+noOutputPath(t))
		require.ErrorContains(t, err, "resolving dependency \"app1\" outputs")
	})

	t.Run("flag rejected without experiment", func(t *testing.T) {
		t.Parallel()

		if helpers.IsExperimentMode(t) {
			t.Skip("Skipping: TG_EXPERIMENT_MODE forces the optional-dependency-outputs experiment on, so its disabled-state error can't be verified")
		}

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --no-dependency-outputs --non-interactive --working-dir "+noOutputPath(t))
		require.ErrorContains(t, err, "--no-dependency-outputs requires the 'optional-dependency-outputs' experiment")
	})

	for _, cmd := range []string{"init", "validate", "plan"} {
		t.Run(cmd+" succeeds with flag", func(t *testing.T) {
			t.Parallel()
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+cmd+" --experiment optional-dependency-outputs --no-dependency-outputs --non-interactive --working-dir "+noOutputPath(t))
			require.NoError(t, err)
		})
	}

	for _, cmd := range []string{"init", "validate", "plan"} {
		t.Run("run --all "+cmd+" succeeds with flag", func(t *testing.T) {
			t.Parallel()
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --experiment optional-dependency-outputs "+cmd+" --no-dependency-outputs --non-interactive --working-dir "+noOutputPath(t))
			require.NoError(t, err)
		})
	}
}
