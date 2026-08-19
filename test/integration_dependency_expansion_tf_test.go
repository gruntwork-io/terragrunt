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

func TestTFDependencyExpansionResolvesKeyedOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyExpansionKeyed)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyExpansionKeyed)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDependencyExpansionKeyed)
	appPath := filepath.Join(rootPath, "app")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all apply --experiment block-iteration --non-interactive --report-file "+
			helpers.ReportFile+" --working-dir "+rootPath+" -- -auto-approve",
	)

	runs := helpers.ReadReport(t, rootPath, helpers.ReportFile)

	dependencies := []string{"vpc", "aurora-web", "aurora-api", "shard-0", "shard-1"}
	assert.ElementsMatch(t, append([]string{"app"}, dependencies...), runs.Names())

	app := runs.FindByName("app")
	require.NotNil(t, app)
	assert.Equal(t, "succeeded", app.Result)

	for _, name := range dependencies {
		run := runs.FindByName(name)

		require.NotNilf(t, run, "%s never ran", name)
		assert.Equalf(t, "succeeded", run.Result, "%s did not succeed", name)
		assert.Nilf(t, run.Reason, "%s carried a reason, so it did not run plainly", name)

		assert.Falsef(t, app.Started.Before(run.Ended), "app started before %s finished", name)
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --experiment block-iteration --non-interactive --working-dir "+appPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "vpc-id", outputs["vpc_id"].Value)
	assert.Equal(t, "aurora-web-id", outputs["web_id"].Value)
	assert.Equal(t, "aurora-api-id", outputs["api_id"].Value)
	assert.Equal(t, "shard-0-id", outputs["shard_first"].Value)
	assert.Equal(t, "shard-1-id", outputs["shard_second"].Value)
}

func TestTFDependencyExpansionResolvesPerInstanceMocks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyExpansionMocks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyExpansionMocks)
	appPath := filepath.Join(tmpEnvPath, testFixtureDependencyExpansionMocks, "app")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply --experiment block-iteration --non-interactive --report-file "+
			helpers.ReportFile+" --working-dir "+appPath+" -- -auto-approve",
	)

	runs := helpers.ReadReport(t, appPath, helpers.ReportFile)

	assert.Equal(t, []string{"app"}, runs.Names())

	app := runs.FindByName("app")
	require.NotNil(t, app)
	assert.Equal(t, "succeeded", app.Result)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --experiment block-iteration --non-interactive --working-dir "+appPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "mock-web", outputs["web_id"].Value)
	assert.Equal(t, "mock-api", outputs["api_id"].Value)
}
