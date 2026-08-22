package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureStacksEnabled = "fixtures/stacks/enabled"

// generateEnabledStack copies the fixture and generates, returning the generated
// .terragrunt-stack directory.
func generateEnabledStack(t *testing.T) string {
	t.Helper()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEnabled)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEnabled)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksEnabled, "live")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+rootPath,
	)

	return filepath.Join(rootPath, ".terragrunt-stack")
}

func TestStackEnabledSkipsDisabledUnits(t *testing.T) {
	t.Parallel()

	stackPath := generateEnabledStack(t)

	assert.FileExists(t, filepath.Join(stackPath, "vpc", "terragrunt.hcl"))

	assert.NoDirExists(t, filepath.Join(stackPath, "legacy"))

	// The shard unit is expanded and disabled, so none of its elements generate.
	assert.NoDirExists(t, filepath.Join(stackPath, "shard"))
}

// TestStackEnabledAllDisabledGeneratesNothing pins that disabling every unit in a stack
// file generates nothing and succeeds. The emptiness rule that rejects a stack file
// declaring no units reads the blocks a user wrote, not what they resolved to.
func TestStackEnabledAllDisabledGeneratesNothing(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEnabled)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEnabled)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksEnabled, "none-enabled")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+rootPath,
	)

	assert.NoDirExists(t, filepath.Join(rootPath, ".terragrunt-stack"))
}

func TestStackEnabledRequiresExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces all experiments on, opening the gate this test pins shut",
		)
	}

	helpers.CleanupTerraformFolder(t, testFixtureStacksEnabled)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEnabled)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksEnabled, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)

	var enabledErr config.EnabledRequiresExperimentError
	require.ErrorAs(t, err, &enabledErr)
	assert.Equal(t, "unit", enabledErr.BlockType)
	assert.Equal(t, "vpc", enabledErr.BlockLabel)
}
