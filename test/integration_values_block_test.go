package test_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureValuesBlock = "fixtures/values-block"

func TestValuesBlockStackGenerateFillsDefaults(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "valid")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment values-block --working-dir "+stackPath)

	inputs := renderValuesBlockInputs(t, filepath.Join(stackPath, ".terragrunt-stack", "app"))
	assert.Equal(t, map[string]any{
		"name":           "web",
		"instance_count": float64(3),
		"tags":           map[string]any{},
	}, inputs)
}

func TestValuesBlockStackGenerateNestedStackDefaults(t *testing.T) {
	t.Parallel()

	fixturePath := copyValuesBlockFixture(t)

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(fixturePath).Init(t.Context()))

	stackPath := filepath.Join(fixturePath, "live", "nested")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment values-block --working-dir "+stackPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --no-color --experiment values-block --working-dir "+stackPath,
	)
	require.NoError(t, err)

	unitRel := filepath.Join(".terragrunt-stack", "service", ".terragrunt-stack", "app-dev")
	assert.Contains(t, strings.Fields(stdout), unitRel)

	inputs := renderValuesBlockInputs(t, filepath.Join(stackPath, unitRel))
	assert.Equal(t, map[string]any{
		"name":           "api",
		"instance_count": float64(2),
		"tags":           map[string]any{},
	}, inputs)
}

// copyValuesBlockFixture copies the values-block fixture and returns the path of the copy.
func copyValuesBlockFixture(t *testing.T) string {
	t.Helper()

	return filepath.Join(helpers.CopyEnvironment(t, testFixtureValuesBlock), testFixtureValuesBlock)
}

// renderValuesBlockInputs renders the unit at unitPath as JSON and returns its inputs.
func renderValuesBlockInputs(t *testing.T, unitPath string) map[string]any {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --json --experiment values-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var rendered struct {
		Inputs map[string]any `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	return rendered.Inputs
}
