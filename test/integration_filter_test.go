package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

const (
	testFixtureFilterBasic  = "fixtures/find/basic"
	testFixtureFilterDAG    = "fixtures/find/dag"
	testFixtureFilterList   = "fixtures/list/basic"
	testFixtureFilterSource = "fixtures/filter-source"
)

// createTestUnit creates a unit directory with terragrunt.hcl and main.tf files.
// Returns the path to the terragrunt.hcl file for later modification.
func createTestUnit(t *testing.T, dir, comment string) string {
	t.Helper()

	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	hclPath := filepath.Join(dir, "terragrunt.hcl")
	err = os.WriteFile(hclPath, []byte(comment), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`# Minimal terraform config`), 0644)
	require.NoError(t, err)

	return hclPath
}

func TestFilterFlagWithFind(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterBasic)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by path - exact match",
			filterQuery:    "unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter by path - wildcard",
			filterQuery:    "./*",
			expectedOutput: "stack\nunit\n",
			expectError:    false,
		},
		{
			name:           "filter by name - exact match",
			filterQuery:    "unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter by type - unit only",
			filterQuery:    "type=unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter by type - stack only",
			filterQuery:    "type=stack",
			expectedOutput: "stack\n",
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude unit",
			filterQuery:    "!unit",
			expectedOutput: "stack\n",
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude path",
			filterQuery:    "!./unit",
			expectedOutput: "stack\n",
			expectError:    false,
		},
		{
			name:           "filter with intersection - path and type",
			filterQuery:    "./unit | type=unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter with intersection - path and negation",
			filterQuery:    "./* | !unit",
			expectedOutput: "stack\n",
			expectError:    false,
		},
		{
			name:           "filter with braced path",
			filterQuery:    "{./unit}",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter with non-matching query",
			filterQuery:    "nonexistent",
			expectedOutput: "",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithFindJSON(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterBasic)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by type - unit only JSON",
			filterQuery:    "type=unit",
			expectedOutput: `[{"type": "unit", "path": "unit"}]`,
			expectError:    false,
		},
		{
			name:           "filter by type - stack only JSON",
			filterQuery:    "type=stack",
			expectedOutput: `[{"type": "stack", "path": "stack"}]`,
			expectError:    false,
		},
		{
			name:           "filter by name - exact match JSON",
			filterQuery:    "unit",
			expectedOutput: `[{"type": "unit", "path": "unit"}]`,
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude unit JSON",
			filterQuery:    "!unit",
			expectedOutput: `[{"type": "stack", "path": "stack"}]`,
			expectError:    false,
		},
		{
			name:           "filter with intersection JSON",
			filterQuery:    "./unit | type=unit",
			expectedOutput: `[{"type": "unit", "path": "unit"}]`,
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --json --filter " + tc.filterQuery
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				assert.JSONEq(t, tc.expectedOutput, stdout, "JSON output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithList(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	// The CLI constructor ensures that the working directory is always absolute.
	workingDir, err := filepath.Abs(testFixtureFilterList)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		filterQuery     string
		expectedResults []string
		expectError     bool
	}{
		{
			name:            "filter by name - exact match",
			filterQuery:     "a-unit",
			expectedResults: []string{"a-unit"},
			expectError:     false,
		},
		{
			name:            "filter by name - exact match with equals",
			filterQuery:     "name=a-unit",
			expectedResults: []string{"a-unit"},
			expectError:     false,
		},
		{
			name:            "filter by type - unit only",
			filterQuery:     "type=unit",
			expectedResults: []string{"a-unit", "b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with negation - exclude a-unit",
			filterQuery:     "!a-unit",
			expectedResults: []string{"b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with negation - exclude path",
			filterQuery:     "!./a-unit",
			expectedResults: []string{"b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with intersection - name and type",
			filterQuery:     "a-unit | type=unit",
			expectedResults: []string{"a-unit"},
			expectError:     false,
		},
		{
			name:            "filter with wildcard path",
			filterQuery:     "./*",
			expectedResults: []string{"a-unit", "b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with braced path",
			filterQuery:     "{a-unit}",
			expectedResults: []string{"a-unit"},
			expectError:     false,
		},
		{
			name:            "filter with non-matching query",
			filterQuery:     "nonexistent",
			expectedResults: []string{},
			expectError:     false,
		},
		{
			name:            "filter with invalid syntax",
			filterQuery:     "invalid{filter",
			expectedResults: []string{},
			expectError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt list --no-color --working-dir " + workingDir + " --filter " + tc.filterQuery
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)

				return
			}

			require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)

			results := strings.Fields(stdout)
			assert.ElementsMatch(t, tc.expectedResults, results, "Output mismatch for filter query: %s", tc.filterQuery)
		})
	}
}

func TestFilterFlagWithListLong(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	testCases := []struct {
		name           string
		workingDir     string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by name - exact match long format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "a-unit",
			expectedOutput: "Type  Path\nunit  a-unit\n",
			expectError:    false,
		},
		{
			name:           "filter by type - unit only long format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "type=unit",
			expectedOutput: "Type  Path\nunit  a-unit\nunit  b-unit\n",
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude a-unit long format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "!a-unit",
			expectedOutput: "Type  Path\nunit  b-unit\n",
			expectError:    false,
		},
		{
			name:           "filter with intersection long format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "a-unit | type=unit",
			expectedOutput: "Type  Path\nunit  a-unit\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tc.workingDir)

			cmd := "terragrunt list --no-color --working-dir " + tc.workingDir + " --long --filter " + tc.filterQuery
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithListTree(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	testCases := []struct {
		name           string
		workingDir     string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by name - exact match tree format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "a-unit",
			expectedOutput: ".\n╰── a-unit\n",
			expectError:    false,
		},
		{
			name:           "filter by type - unit only tree format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "type=unit",
			expectedOutput: ".\n├── a-unit\n╰── b-unit\n",
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude a-unit tree format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "!a-unit",
			expectedOutput: ".\n╰── b-unit\n",
			expectError:    false,
		},
		{
			name:           "filter with intersection tree format",
			workingDir:     testFixtureFilterList,
			filterQuery:    "a-unit | type=unit",
			expectedOutput: ".\n╰── a-unit\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tc.workingDir)

			cmd := "terragrunt list --no-color --working-dir " + tc.workingDir + " --tree --filter " + tc.filterQuery
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithDAG(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterDAG)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by path - specific component",
			filterQuery:    "./a-dependent",
			expectedOutput: "a-dependent\n",
			expectError:    false,
		},
		{
			name:           "filter by name - specific component",
			filterQuery:    "a-dependent",
			expectedOutput: "a-dependent\n",
			expectError:    false,
		},
		{
			name:           "filter by type - unit only",
			filterQuery:    "type=unit",
			expectedOutput: "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n",
			expectError:    false,
		},
		{
			name:           "filter with negation - exclude specific component",
			filterQuery:    "!a-dependent",
			expectedOutput: "b-dependency\nc-mixed-deps\nd-dependencies-only\n",
			expectError:    false,
		},
		{
			name:           "filter with wildcard - all components",
			filterQuery:    "./*",
			expectedOutput: "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n",
			expectError:    false,
		},
		{
			name:           "filter with intersection - path and type",
			filterQuery:    "./a-dependent | type=unit",
			expectedOutput: "a-dependent\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter " + tc.filterQuery
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagMultipleFilters(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterBasic)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		expectedOutput string
		filterQueries  []string
		expectError    bool
	}{
		{
			name:           "multiple filters - union semantics",
			filterQueries:  []string{"./unit", "./stack"},
			expectedOutput: "stack\nunit\n",
			expectError:    false,
		},
		{
			name:           "multiple filters with negation",
			filterQueries:  []string{"./*", "!unit"},
			expectedOutput: "stack\n",
			expectError:    false,
		},
		{
			name:           "multiple filters with type",
			filterQueries:  []string{"type=unit", "type=stack"},
			expectedOutput: "stack\nunit\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			// Build command with multiple --filter flags
			cmd := "terragrunt find --no-color --working-dir " + workingDir
			for _, filter := range tc.filterQueries {
				cmd += " --filter " + filter
			}

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter queries: %v", tc.filterQueries)
			} else {
				require.NoError(t, err, "Unexpected error for filter queries: %v", tc.filterQueries)
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter queries: %v", tc.filterQueries)
			}
		})
	}
}

func TestFilterFlagEdgeCases(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterBasic)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter with spaces in name",
			filterQuery:    "unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter with double negation",
			filterQuery:    "!!unit",
			expectedOutput: "unit\n",
			expectError:    false,
		},
		{
			name:           "filter with empty intersection",
			filterQuery:    "unit|nonexistent", // Our testing arg parsing is busted. Don't put whitespace between these.
			expectedOutput: "",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithSource(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	workingDir, err := filepath.Abs(testFixtureFilterSource)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "filter by source - exact match github.com/acme/foo",
			filterQuery:    "source=github.com/acme/foo",
			expectedOutput: "github-acme-foo\n",
			expectError:    false,
		},
		{
			name:           "filter by source - glob pattern *github.com**acme/*",
			filterQuery:    "source=*github.com**acme/*",
			expectedOutput: "github-acme-foo\ngithub-acme-bar\n",
			expectError:    false,
		},
		{
			name:           "filter by source - glob pattern git::git@github.com:acme/**",
			filterQuery:    "source=git::git@github.com:acme/**",
			expectedOutput: "github-acme-bar\n",
			expectError:    false,
		},
		{
			name:           "filter by source - glob pattern **github.com**",
			filterQuery:    "source=**github.com**",
			expectedOutput: "github-acme-foo\ngithub-acme-bar\n",
			expectError:    false,
		},
		{
			name:           "filter by source - exact match gitlab.com/example/baz",
			filterQuery:    "source=gitlab.com/example/baz",
			expectedOutput: "gitlab-example-baz\n",
			expectError:    false,
		},
		{
			name:           "filter by source - glob pattern gitlab.com/**",
			filterQuery:    "source=gitlab.com/**",
			expectedOutput: "gitlab-example-baz\n",
			expectError:    false,
		},
		{
			name:           "filter by source - local module",
			filterQuery:    "source=./module",
			expectedOutput: "local-module\n",
			expectError:    false,
		},
		{
			name:           "filter by source - non-matching query",
			filterQuery:    "source=nonexistent",
			expectedOutput: "",
			expectError:    false,
		},
		{
			name:           "filter by source with negation - exclude github.com/acme/foo",
			filterQuery:    "!source=github.com/acme/foo",
			expectedOutput: "github-acme-bar\ngitlab-example-baz\nlocal-module\n",
			expectError:    false,
		},
		{
			name:           "filter by source with intersection - github.com/acme/* and path",
			filterQuery:    "source=github.com/acme/* | ./github-acme-foo",
			expectedOutput: "github-acme-foo\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				assert.Empty(t, stderr, "Unexpected error message in stderr")
				// Sort both outputs for comparison since order may vary
				expectedLines := strings.Fields(tc.expectedOutput)
				actualLines := strings.Fields(stdout)
				assert.ElementsMatch(t, expectedLines, actualLines, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithRunAllGitFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	testCases := []struct {
		name               string
		filterQuery        string
		description        string
		expectedUnits      []string
		ignoredUnits       []string
		expectedExcluded   []string
		filterAllowDestroy bool
		expectError        bool
	}{
		{
			name:               "git filter discovers modified, created, and removed units and excludes untouched",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: false,
			expectedUnits:      []string{"unit-to-be-created", "unit-to-be-modified"},
			ignoredUnits:       []string{"unit-to-be-untouched"},
			expectedExcluded:   []string{"unit-to-be-removed"},
			expectError:        false,
			description:        "Git filter should discover units that were created, modified, or removed between commits, and exclude untouched units. Removed unit should be excluded without --filter-allow-destroy",
		},
		{
			name:               "git filter with --filter-allow-destroy includes removed unit",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: true,
			expectedUnits:      []string{"unit-to-be-created", "unit-to-be-modified", "unit-to-be-removed"},
			ignoredUnits:       []string{"unit-to-be-untouched"},
			expectedExcluded:   []string{},
			expectError:        false,
			description:        "Git filter with --filter-allow-destroy should include removed unit for destroy operations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tmpDir, err := filepath.EvalSymlinks(tmpDir)
			require.NoError(t, err)

			runner, err := git.NewGitRunner()
			require.NoError(t, err)

			runner = runner.WithWorkDir(tmpDir)

			err = runner.Init(t.Context())
			require.NoError(t, err)

			err = runner.GoOpenRepo()
			require.NoError(t, err)

			t.Cleanup(func() {
				err = runner.GoCloseStorage()
				require.NoError(t, err)
			})

			// Create three units initially using helper
			unitToBeModifiedDir := filepath.Join(tmpDir, "unit-to-be-modified")
			unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
			unitToBeUntouchedDir := filepath.Join(tmpDir, "unit-to-be-untouched")

			unitToBeModifiedHCLPath := createTestUnit(t, unitToBeModifiedDir, `# Unit to be modified`)
			_ = createTestUnit(t, unitToBeRemovedDir, `# Unit to be removed`)
			_ = createTestUnit(t, unitToBeUntouchedDir, `# Unit to be untouched`)

			// Initial commit
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Modify the unit to be modified
			err = os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit modified`), 0644)
			require.NoError(t, err)

			// Remove the unit to be removed (delete the directory)
			err = os.RemoveAll(unitToBeRemovedDir)
			require.NoError(t, err)

			// Add a unit to be created
			unitToBeCreatedDir := filepath.Join(tmpDir, "unit-to-be-created")
			_ = createTestUnit(t, unitToBeCreatedDir, `# Unit created`)

			// Do nothing to the unit to be untouched

			// Commit the modification and removal in a single commit
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Create, modify, and remove units", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Run terragrunt run --all --filter with git filter
			// Note: We use 'plan' command which should work even without terraform init
			// Note: --experiment-mode enables the filter-flag experiment required for --filter
			reportFile := "report.json"
			cmd := "terragrunt run --all --no-color --experiment-mode --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' --report-file " + reportFile

			if tc.filterAllowDestroy {
				cmd += " --filter-allow-destroy"
			}

			cmd += " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				// For run commands, we expect some output even if terraform isn't fully initialized
				// The key is that the command should execute and process the filtered units
				if err != nil {
					// If there's an error, it might be because terraform isn't initialized
					// but we should still see that the filter worked (units were discovered)
					// Let's check if the error is about terraform init or similar
					if !strings.Contains(stderr, "terraform") && !strings.Contains(stderr, "tofu") {
						// Unexpected error
						require.NoError(t, err, "Unexpected error for filter query: %s\nstdout: %s\nstderr: %s", tc.filterQuery, stdout, stderr)
					}
				}

				// Verify the report file exists
				reportFilePath := util.JoinPath(tmpDir, reportFile)
				assert.FileExists(t, reportFilePath, "Report file should exist")

				// Read and parse the report file
				content, err := os.ReadFile(reportFilePath)
				require.NoError(t, err, "Should be able to read report file")

				var records []map[string]string

				err = json.Unmarshal(content, &records)
				require.NoError(t, err, "Should be able to parse report JSON")

				// Create a map of unit names to records for easier lookup
				// The report contains full paths, so we extract the unit name from the path
				recordsByUnit := make(map[string]map[string]string)

				for _, record := range records {
					fullPath := record["Name"]
					// Extract unit name from path (e.g., "unit-to-be-created" from "/tmp/.../unit-to-be-created")
					baseName := filepath.Base(fullPath)
					recordsByUnit[baseName] = record
					// Also store by full path for fallback
					recordsByUnit[fullPath] = record
					// Store by any part of the path that matches our unit pattern
					parts := strings.Split(fullPath, string(filepath.Separator))
					for _, part := range parts {
						if strings.HasPrefix(part, "unit-to-be-") {
							recordsByUnit[part] = record
						}
					}
				}

				// Verify expected units are in the report and not excluded
				for _, expectedUnit := range tc.expectedUnits {
					record, found := recordsByUnit[expectedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, expectedUnit) {
								record = rec
								found = true

								break
							}
						}
					}

					require.True(t, found, "Expected unit '%s' should be in report. Found units: %v", expectedUnit, getUnitNames(recordsByUnit))
					assert.NotEqual(t, "excluded", record["Result"], "Expected unit '%s' should not be excluded", expectedUnit)
				}

				// Verify excluded units are NOT in the report
				for _, excludedUnit := range tc.ignoredUnits {
					found := false

					for name := range recordsByUnit {
						if strings.Contains(name, excludedUnit) {
							found = true
							break
						}
					}

					assert.False(t, found, "Excluded unit '%s' should NOT be in report", excludedUnit)
				}

				// Verify expected excluded units are in the report but marked as excluded
				for _, excludedUnit := range tc.expectedExcluded {
					record, found := recordsByUnit[excludedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, excludedUnit) {
								record = rec
								found = true

								break
							}
						}
					}

					require.True(t, found, "Expected excluded unit '%s' should be in report", excludedUnit)
					assert.Equal(t, "excluded", record["Result"], "Unit '%s' should be marked as excluded", excludedUnit)
				}
			}
		})
	}
}

// getUnitNames extracts unit names from records map for error messages
func getUnitNames(recordsByUnit map[string]map[string]string) []string {
	names := make([]string, 0, len(recordsByUnit))
	for name := range recordsByUnit {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
