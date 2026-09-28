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
	testFixtureMarkManyAsReadRelpath = "fixtures/mark-many-as-read-relpath"
	testFixtureMarkGlobAsRead        = "fixtures/mark-glob-as-read"
)

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
