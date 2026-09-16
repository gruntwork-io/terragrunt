//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureNoDependencyOutputs = "fixtures/no-dependency-outputs"

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

	for _, cmd := range []string{"init", "validate", "plan"} {
		t.Run(cmd+" succeeds with flag", func(t *testing.T) {
			t.Parallel()
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+cmd+" --no-dependency-outputs --non-interactive --working-dir "+noOutputPath(t))
			require.NoError(t, err)
		})
	}

	for _, cmd := range []string{"init", "validate", "plan"} {
		t.Run("run --all "+cmd+" succeeds with flag", func(t *testing.T) {
			t.Parallel()
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all "+cmd+" --no-dependency-outputs --non-interactive --working-dir "+noOutputPath(t))
			require.NoError(t, err)
		})
	}
}

func TestTFDependencyOutputSkipDependencyOutputsFlagWithApplyAndDestroy(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoDependencyOutputs)

	testCases := []struct {
		want map[string]string
		name string
		args string
	}{
		{
			name: "with flag",
			args: " --no-dependency-outputs",
			want: map[string]string{
				"dep":               "real",
				"consumer":          "",
				"mocks-allowed":     "mocked",
				"mocks-not-allowed": "",
			},
		},
		{
			name: "without flag",
			want: map[string]string{
				"dep":               "real",
				"consumer":          "real",
				"mocks-allowed":     "real",
				"mocks-not-allowed": "real",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDependencyOutputs)
			rootPath := filepath.Join(tmpEnvPath, testFixtureNoDependencyOutputs)

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply"+tc.args+" --non-interactive --working-dir "+rootPath)
			require.NoError(t, err)

			for unit, want := range tc.want {
				stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -json"+tc.args+" --non-interactive --working-dir "+filepath.Join(rootPath, unit))
				require.NoError(t, err)

				outputs := map[string]helpers.TerraformOutput{}
				require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
				helpers.ValidateOutput(t, outputs, "x", want)
			}

			_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all destroy"+tc.args+" --non-interactive --working-dir "+rootPath)
			require.NoError(t, err)
		})
	}
}
