//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFPathVariableModuleIsRunDirectory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit string
	}{
		{name: "download_dir in the unit", unit: "download-dir"},
		{name: "download_dir in an included file", unit: "download-dir-include"},
		{name: "in-place run", unit: "in-place"},
		{name: "source in terraform block", unit: "remote"},
		{name: "no source", unit: "local"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := copyPathVariableFixture(t)
			flags := " --non-interactive --working-dir " + filepath.Join(root, "module", tc.unit) +
				pathVariableExperimentFlag + " --experiment no-dot-terragrunt-cache"

			helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -json"+flags)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

			module, ok := outputs["module"].Value.(string)
			require.True(t, ok)

			assert.Equal(t, outputs["run_dir"].Value, filepath.ToSlash(module))
		})
	}
}

func TestTFPathVariableInDependencyOfUnitThatStackReads(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackDir := filepath.Join(root, "stack-read-dependency", "live")
	unitDir := filepath.Join(stackDir, ".terragrunt-stack", "app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+pathVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"app_stack":      stackDir,
		"app_db":         "mock",
		"auto_app_stack": stackDir,
	}, renderPathVariableInputs(t, unitDir))
}

func TestTFPathVariableUnitInAwsProviderPatchAll(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)

	workingDir := filepath.Join(root, "all")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt aws-provider-patch --all --override-attr region=eu-west-1 --non-interactive --working-dir "+
			workingDir+pathVariableExperimentFlag,
	)
	require.NoError(t, err)

	initialized, err := filepath.Glob(filepath.Join(workingDir, "*", ".terragrunt-cache", "*", "*", ".terraform"))
	require.NoError(t, err)
	assert.Len(t, initialized, 2)
	assert.NoDirExists(t, filepath.Join(workingDir, ".terragrunt-cache"))
}
