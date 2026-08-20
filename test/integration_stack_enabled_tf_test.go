//go:build tf

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

// TestTFStackEnabledLeavesOutputsAddressable pins that a disabled unit drops out of stack
// output without taking the rest of the stack with it. Without the skip, reading a
// never-generated unit's outputs fails the whole command.
func TestTFStackEnabledLeavesOutputsAddressable(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEnabled)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEnabled)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksEnabled, "live")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack run apply --experiment block-iteration --non-interactive --working-dir "+
			rootPath+" -- -auto-approve",
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output --experiment block-iteration --non-interactive --working-dir "+
			rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "vpc")
	assert.NotContains(t, stdout, "legacy")
	assert.NotContains(t, stdout, "shard")
}
