package test_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureMarkManyAsReadRelpath = "fixtures/mark-many-as-read-relpath"

func TestMarkManyAsReadRelpathSourceTriggersDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMarkManyAsReadRelpath)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkManyAsReadRelpath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkManyAsReadRelpath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive --experiment mark-many-as-read "+
			"--queue-include-units-reading=modules/foo/main.tf --report-file %s "+
			"--working-dir %s -- plan",
		helpers.ReportFile, rootPath,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)

	assert.NotContains(t, stdout+stderr, "No units discovered",
		"unit should be discovered via mark-many-as-read source walk")

	runs, err := report.ParseJSONRunsFromFile(filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"live/unit"}, runs.Names())
}

func TestMarkManyAsReadDisabledDoesNotDiscoverModuleChanges(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping: TG_EXPERIMENT_MODE forces all experiments on, defeating the disabled-vs-enabled comparison this test pins")
	}

	helpers.CleanupTerraformFolder(t, testFixtureMarkManyAsReadRelpath)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkManyAsReadRelpath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkManyAsReadRelpath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive "+
			"--queue-include-units-reading=modules/foo/main.tf --report-file %s "+
			"--working-dir %s -- plan",
		helpers.ReportFile, rootPath,
	)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	runs, err := report.ParseJSONRunsFromFile(filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.Empty(t, runs.Names(),
		"without mark-many-as-read, module source changes should not cascade to the unit")
}
