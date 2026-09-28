package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureMarkManyAsReadRelpath        = "fixtures/mark-many-as-read-relpath"
	testFixtureMarkManyAsReadIncludedSource = "fixtures/mark-many-as-read-included-source"
	testFixtureMarkGlobAsRead               = "fixtures/mark-glob-as-read"
)

// TestMarkManyAsReadIncludedRelativeSource pins that a relative terraform source
// inherited through include resolves against the unit, where a run resolves it,
// so a reading= filter naming the module file selects the unit.
func TestMarkManyAsReadIncludedRelativeSource(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(testFixtureMarkManyAsReadIncludedSource)
	require.NoError(t, err)

	helpers.CleanupTerraformFolder(t, workingDir)

	cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter 'reading=modules/foo/main.tf'"
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stderr: %s", stderr)

	assert.ElementsMatch(t, []string{"live/unit"}, strings.Fields(stdout))
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
