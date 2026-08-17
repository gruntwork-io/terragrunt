package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureStacksExpansion = "fixtures/stacks/expansion"

// generateExpansionStack copies the fixture and generates, returning the generated
// .terragrunt-stack directory.
func generateExpansionStack(t *testing.T) string {
	t.Helper()

	helpers.CleanupTerraformFolder(t, testFixtureStacksExpansion)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksExpansion)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksExpansion, "live")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+rootPath,
	)

	return filepath.Join(rootPath, ".terragrunt-stack")
}

func TestStackExpansionGeneratesOneDirectoryPerElement(t *testing.T) {
	t.Parallel()

	stackPath := generateExpansionStack(t)

	for _, unit := range []string{
		filepath.Join("aurora", "web"),
		filepath.Join("aurora", "api"),
		filepath.Join("shard", "0"),
		filepath.Join("shard", "1"),
		"vpc",
	} {
		assert.FileExists(t, filepath.Join(stackPath, unit, "terragrunt.hcl"))
	}

	// A nested stack generates its own .terragrunt-stack, so its units sit a level deeper.
	for _, stack := range []string{"0", "1"} {
		assert.FileExists(
			t,
			filepath.Join(stackPath, "team", stack, ".terragrunt-stack", "member", "terragrunt.hcl"),
		)
	}
}

func TestStackExpansionResolvesValuesPerInstance(t *testing.T) {
	t.Parallel()

	stackPath := generateExpansionStack(t)

	roles := map[string]string{
		filepath.Join("aurora", "web"): "web",
		filepath.Join("aurora", "api"): "api",
		filepath.Join("shard", "0"):    "shard-0",
		filepath.Join("shard", "1"):    "shard-1",
		"vpc":                          "vpc",
	}

	for unit, role := range roles {
		values, err := os.ReadFile(filepath.Join(stackPath, unit, "terragrunt.values.hcl"))
		require.NoError(t, err)

		assert.Contains(t, string(values), `role = "`+role+`"`)
	}
}

func TestStackExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces all experiments on, opening the gate this test pins shut",
		)
	}

	helpers.CleanupTerraformFolder(t, testFixtureStacksExpansion)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksExpansion)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksExpansion, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)

	var expansionErr config.ExpansionRequiresExperimentError
	require.ErrorAs(t, err, &expansionErr)
	assert.Equal(t, "unit", expansionErr.BlockType)
}
