package test_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureMarkManyAsReadRelpath = "fixtures/mark-many-as-read-relpath"
	testFixtureMarkGlobAsRead        = "fixtures/mark-glob-as-read"
)

func TestMarkManyAsReadRelpathSourceTriggersDiscovery(t *testing.T) {
	t.Parallel()

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

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)

	assert.NotContains(t, stdout+stderr, "No units discovered",
		"unit should be discovered via the local module source walk")

	runs, err := report.ParseJSONRunsFromFile(filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"live/unit"}, runs.Names())
}

// TestMarkManyAsReadDefaultDiscoversModuleChanges pins that the local module
// source walk is on by default: a reading= filter naming a module file selects
// the unit consuming that module, with no experiment flag.
func TestMarkManyAsReadDefaultDiscoversModuleChanges(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMarkManyAsReadRelpath)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkManyAsReadRelpath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkManyAsReadRelpath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive "+
			"--filter 'reading=modules/foo/main.tf' --report-file %s "+
			"--working-dir %s -- plan",
		helpers.ReportFile, rootPath,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)

	runs, err := report.ParseJSONRunsFromFile(filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"live/unit"}, runs.Names(),
		"module source changes should cascade to the unit by default")
}

// TestMarkGlobAsReadReadingFilter exercises mark_glob_as_read() end-to-end:
// the unit's terragrunt.hcl globs a sibling data file, and a reading= filter
// matching that file selects the unit. A data file that no unit marks as read
// matches nothing.
func TestMarkGlobAsReadReadingFilter(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(testFixtureMarkGlobAsRead)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		filterQuery   string
		expectedUnits []string
	}{
		{
			name:          "exact path to the globbed data file selects the unit",
			filterQuery:   "reading=unit/settings.yaml",
			expectedUnits: []string{"unit"},
		},
		{
			name:          "glob filter matching the globbed data file selects the unit",
			filterQuery:   "reading=*/settings.yaml",
			expectedUnits: []string{"unit"},
		},
		{
			name:          "data file not marked by any unit selects nothing",
			filterQuery:   "reading=*/data.yaml",
			expectedUnits: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err, "stderr: %s", stderr)

			assert.ElementsMatch(t, tc.expectedUnits, strings.Fields(stdout),
				"output mismatch for filter query: %s", tc.filterQuery)
		})
	}
}
