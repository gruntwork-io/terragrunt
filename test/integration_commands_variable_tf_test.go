//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFCommandsVariableSelectsHookAndInputs(t *testing.T) {
	t.Parallel()

	root := copyCommandsVariableFixture(t)
	unitDir := filepath.Join(root, "live", "app")
	flags := " --non-interactive --working-dir " + unitDir + commandsVariableExperimentFlag

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

	require.FileExists(t, filepath.Join(unitDir, "hook.out"))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -json"+flags)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	require.Equal(t, commandList(config.TerraformCommandsNeedVars), outputs["vars"].Value)
	require.Equal(t, commandList(config.TerraformCommandsNeedLocking), outputs["root_locking"].Value)
}
