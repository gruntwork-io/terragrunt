package test_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureFilterBasic             = "fixtures/find/basic"
	testFixtureFilterDAG               = "fixtures/find/dag"
	testFixtureFilterList              = "fixtures/list/basic"
	testFixtureFilterSource            = "fixtures/filter-source"
	testFixtureMinimizeParsing         = "fixtures/filter/minimize-parsing"
	testFixtureMinimizeParsingDestroy  = "fixtures/filter/minimize-parsing-destroy"
	testFixtureExcludeByDefault        = "fixtures/exclude-by-default"
	testFixtureFilterMarkAsRead        = "fixtures/filter/mark-as-read"
	testFixtureFilterMarkAsReadInclude = "fixtures/filter/mark-as-read-include"
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
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithFindJSON(t *testing.T) {
	t.Parallel()

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
				assert.JSONEq(
					t,
					tc.expectedOutput,
					stdout,
					"JSON output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithList(t *testing.T) {
	t.Parallel()

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
			// Quoted so that shellwords keeps the pipe in the filter rather than cutting the
			// command short at it.
			name:            "filter with intersection - name and type",
			filterQuery:     "'a-unit | type=unit'",
			expectedResults: []string{"a-unit"},
			expectError:     false,
		},
		{
			name:            "filter with intersection - negated name and type",
			filterQuery:     "'!a-unit | type=unit'",
			expectedResults: []string{"b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with intersection - negated name and name",
			filterQuery:     "'!a-unit | b-unit'",
			expectedResults: []string{"b-unit"},
			expectError:     false,
		},
		{
			name:            "filter with intersection - operands that share nothing",
			filterQuery:     "'!a-unit | a-unit'",
			expectedResults: []string{},
			expectError:     false,
		},
		{
			name:            "filter with intersection - both operands negated",
			filterQuery:     "'!a-unit | !b-unit'",
			expectedResults: []string{},
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
			assert.ElementsMatch(
				t,
				tc.expectedResults,
				results,
				"Output mismatch for filter query: %s",
				tc.filterQuery,
			)
		})
	}
}

func TestFilterFlagWithListLong(t *testing.T) {
	t.Parallel()

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
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithListTree(t *testing.T) {
	t.Parallel()

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
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithDAG(t *testing.T) {
	t.Parallel()

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
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagMultipleFilters(t *testing.T) {
	t.Parallel()

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

			var cmdSb551 strings.Builder

			for _, filter := range tc.filterQueries {
				cmdSb551.WriteString(" --filter " + filter)
			}

			cmd += cmdSb551.String()

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter queries: %v", tc.filterQueries)
			} else {
				require.NoError(t, err, "Unexpected error for filter queries: %v", tc.filterQueries)
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter queries: %v",
					tc.filterQueries,
				)
			}
		})
	}
}

func TestFilterFlagEdgeCases(t *testing.T) {
	t.Parallel()

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
				assert.Equal(
					t,
					tc.expectedOutput,
					stdout,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithSource(t *testing.T) {
	t.Parallel()

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
				assert.ElementsMatch(
					t,
					expectedLines,
					actualLines,
					"Output mismatch for filter query: %s",
					tc.filterQuery,
				)
			}
		})
	}
}

func TestFilterFlagWithFindGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// Create three units initially
	unitToBeModifiedDir := filepath.Join(tmpDir, "unit-to-be-modified")
	unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
	unitToBeUntouchedDir := filepath.Join(tmpDir, "unit-to-be-untouched")

	err := os.MkdirAll(unitToBeModifiedDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(unitToBeRemovedDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(unitToBeUntouchedDir, 0755)
	require.NoError(t, err)

	// Create minimal terragrunt.hcl files for each unit
	unitToBeModifiedHCLPath := filepath.Join(unitToBeModifiedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit to be modified`), 0644)
	require.NoError(t, err)

	unitToBeRemovedHCLPath := filepath.Join(unitToBeRemovedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeRemovedHCLPath, []byte(`# Unit to be removed`), 0644)
	require.NoError(t, err)

	unitToBeUntouchedHCLPath := filepath.Join(unitToBeUntouchedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeUntouchedHCLPath, []byte(`# Unit to be untouched`), 0644)
	require.NoError(t, err)

	// Initial commit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// If users don't have a default branch set, we'll make sure that the `main` branch exists
	b, err := runner.Config(t.Context(), "init.defaultBranch")
	if err != nil || b != "main" {
		require.NoError(t, runner.Checkout(t.Context(), "main", true))
	}

	// We'll checkout a new branch so that we can compare against main in the filter-affected flag test
	require.NoError(t, runner.Checkout(t.Context(), "filter-affected-test", true))

	// Modify the unit to be modified
	err = os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit modified`), 0644)
	require.NoError(t, err)

	// Remove the unit to be removed (delete the directory)
	err = os.RemoveAll(unitToBeRemovedDir)
	require.NoError(t, err)

	// Add a unit to be created
	unitToBeCreatedDir := filepath.Join(tmpDir, "unit-to-be-created")
	err = os.MkdirAll(unitToBeCreatedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(unitToBeCreatedDir, "terragrunt.hcl"),
		[]byte(`# Unit created`),
		0644,
	)
	require.NoError(t, err)

	// Do nothing to the unit to be untouched

	// Commit the modification and removal in a single commit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Create, modify, and remove units"))

	// Clean up terraform folders before running
	helpers.CleanupTerraformFolder(t, tmpDir)

	testCases := []struct {
		name                  string
		filterQuery           string
		expectedUnits         []string
		useFilterAffectedFlag bool
		expectError           bool
	}{
		{
			name:        "standard git filter",
			filterQuery: "[HEAD~1...HEAD]",
			expectedUnits: []string{
				"unit-to-be-created",
				"unit-to-be-modified",
				"unit-to-be-removed",
			},
			expectError: false,
		},
		{
			name: "filter-affected flag",
			expectedUnits: []string{
				"unit-to-be-created",
				"unit-to-be-modified",
				"unit-to-be-removed",
			},
			useFilterAffectedFlag: true,
			expectError:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helpers.CleanupTerraformFolder(t, tmpDir)

			cmd := "terragrunt find --no-color --working-dir " + tmpDir
			if tc.useFilterAffectedFlag {
				cmd += " --filter-affected"
			}

			if tc.filterQuery != "" {
				cmd += " --filter '" + tc.filterQuery + "'"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")

				return
			}

			results := strings.Split(strings.TrimSpace(stdout), "\n")
			assert.ElementsMatch(t, tc.expectedUnits, results)
		})
	}
}

func TestFilterFlagWithFindGitFilterRelativeInclude(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// Create a root.hcl at the repo root that the nested unit will include
	rootHCLPath := filepath.Join(tmpDir, "root.hcl")
	err := os.WriteFile(rootHCLPath, []byte(`# Root config
`), 0644)
	require.NoError(t, err)

	// Create a deeply nested unit that uses get_path_to_repo_root() in its include path
	nestedUnitDir := filepath.Join(tmpDir, "level1", "level2", "level3", "nested-unit")
	err = os.MkdirAll(nestedUnitDir, 0755)
	require.NoError(t, err)

	nestedUnitHCLPath := filepath.Join(nestedUnitDir, "terragrunt.hcl")
	err = os.WriteFile(nestedUnitHCLPath, []byte(`include "root" {
  path = "${get_path_to_repo_root()}/root.hcl"
}
`), 0644)
	require.NoError(t, err)

	// Initial commit on main
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// Ensure the main branch exists
	b, err := runner.Config(t.Context(), "init.defaultBranch")
	if err != nil || b != "main" {
		require.NoError(t, runner.Checkout(t.Context(), "main", true))
	}

	// Create a feature branch
	require.NoError(t, runner.Checkout(t.Context(), "relative-include-test", true))

	// Modify the nested unit
	err = os.WriteFile(nestedUnitHCLPath, []byte(`include "root" {
  path = "${get_path_to_repo_root()}/root.hcl"
}

# Modified on feature branch
`), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Modify nested unit"))

	helpers.CleanupTerraformFolder(t, tmpDir)

	cmd := "terragrunt find --no-color --working-dir " + tmpDir + " --filter '[main...HEAD]'"
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

	require.NoError(t, err, "terragrunt find with git filter failed: %s", stderr)

	results := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.ElementsMatch(t, []string{"level1/level2/level3/nested-unit"}, results)
}

func TestFilterFlagWithRunAllGitFilterLocalStateWarning(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		unitConfig    string
		description   string
		expectWarning bool
	}{
		{
			name:          "warning fires when unit has no remote_state",
			unitConfig:    `# Unit with no remote_state`,
			expectWarning: true,
			description:   "Warning should fire when unit discovered via Git ref has no remote_state configuration",
		},
		{
			name: "warning fires when unit has local backend",
			unitConfig: `remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}
# Unit with local backend`,
			expectWarning: true,
			description:   "Warning should fire when unit discovered via Git ref has local backend",
		},
		{
			name: "no warning when unit has remote state backend",
			unitConfig: `remote_state {
  backend = "s3"
  config = {
    bucket = "test-bucket"
    key    = "terraform.tfstate"
    region = "us-east-1"
  }
}
# Unit with remote state`,
			expectWarning: false,
			description:   "Warning should not fire when unit discovered via Git ref has remote state backend",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tmpDir, err := filepath.EvalSymlinks(tmpDir)
			require.NoError(t, err)

			runner := helpers.InitTestGitRunner(t, tmpDir)

			// Create a unit with the specified configuration
			unitDir := filepath.Join(tmpDir, "test-unit")
			unitHCLPath := createTestUnit(t, unitDir, tc.unitConfig)

			// Initial commit
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

			// Modify the unit to trigger Git filter detection
			err = os.WriteFile(unitHCLPath, []byte(tc.unitConfig+"\n# Modified"), 0644)
			require.NoError(t, err)

			// Commit the modification
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Modify unit"))

			// Run terragrunt run --all --filter with git filter
			cmd := "terragrunt run --all --no-color --working-dir " + tmpDir + " --filter '[HEAD~1...HEAD]' --report-file " + helpers.ReportFile + " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			// Check for the warning in stderr
			// The warning message should contain this unique substring
			warningMessage := "do not have a remote_state configuration"
			hasWarning := strings.Contains(stderr, warningMessage) &&
				strings.Contains(stderr, "Git-based filter expressions")

			if tc.expectWarning {
				assert.True(
					t,
					hasWarning,
					"Expected warning message in stderr. stderr: %s\nstdout: %s",
					stderr,
					stdout,
				)
			} else {
				assert.False(
					t,
					hasWarning,
					"Did not expect warning message in stderr. stderr: %s\nstdout: %s",
					stderr,
					stdout,
				)
			}

			// The command may fail due to the backend not being bootstrapped, but that's okay.
			// We're just checking for the warning
			_ = err
		})
	}
}

func TestFiltersFileFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setupFile     func(t *testing.T, dir string) string // Returns path to filter file, empty if no file
		cmdFlags      string                                // Additional flags like --filters-file or --no-filters-file
		expectedUnits []string
		expectError   bool
	}{
		{
			name: "custom filters file with --filters-file flag",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, "custom-filters.txt")
				err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "", // Will be set in test
			expectedUnits: []string{"unit"},
			expectError:   false,
		},
		{
			name: "default .terragrunt-filters file is automatically read when experiment enabled",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, ".terragrunt-filters")
				err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "",               // No flag, should auto-detect and read .terragrunt-filters
			expectedUnits: []string{"unit"}, // Should filter to only unit, proving file was read
			expectError:   false,
		},
		{
			name: "--no-filters-file disables auto-reading",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, ".terragrunt-filters")
				err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "--no-filters-file",
			expectedUnits: []string{"stack", "unit"}, // Should show all units, not filtered
			expectError:   false,
		},
		{
			name: "filter file with comments and empty lines",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, ".terragrunt-filters")
				content := "# This is a comment\n\ntype=unit\n  \n# Another comment\n"
				err := os.WriteFile(filterFile, []byte(content), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "",
			expectedUnits: []string{"unit"},
			expectError:   false,
		},
		{
			name: "multiple filters in file",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, ".terragrunt-filters")
				content := "unit\nstack\n"
				err := os.WriteFile(filterFile, []byte(content), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "",
			expectedUnits: []string{"stack", "unit"}, // Union of both filters
			expectError:   false,
		},
		{
			name: "filters file combined with --filter flags",
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()

				filterFile := filepath.Join(dir, ".terragrunt-filters")
				err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
				require.NoError(t, err)

				return filterFile
			},
			cmdFlags:      "--filter type=stack",
			expectedUnits: []string{"stack", "unit"}, // Union: file has unit, flag has stack
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Copy fixture to temporary directory
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterBasic)
			tmpDir := filepath.Join(tmpEnvPath, testFixtureFilterBasic)

			helpers.CleanupTerraformFolder(t, tmpDir)

			// Setup filter file if needed
			var filterFilePath string
			if tc.setupFile != nil {
				filterFilePath = tc.setupFile(t, tmpDir)
			}

			// Build command
			cmd := "terragrunt find --no-color --working-dir " + tmpDir
			if tc.cmdFlags != "" {
				cmd += " " + tc.cmdFlags
			}
			// For custom filter files (not .terragrunt-filters), add --filters-file flag
			if filterFilePath != "" && filepath.Base(filterFilePath) != ".terragrunt-filters" &&
				!strings.Contains(tc.cmdFlags, "--filters-file") &&
				!strings.Contains(tc.cmdFlags, "--no-filters-file") {
				cmd += " --filters-file " + filterFilePath
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for test case: %s", tc.name)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")

				return
			}

			require.NoError(
				t,
				err,
				"Unexpected error for test case: %s\nstdout: %s\nstderr: %s",
				tc.name,
				stdout,
				stderr,
			)
			// Parse output into unit names (split by newlines and filter empty strings)
			results := strings.Split(strings.TrimSpace(stdout), "\n")
			// Filter out empty strings and extract basename from each path
			var actualUnits []string

			for _, r := range results {
				if r != "" {
					// Extract basename from path (handles both relative and absolute paths)
					unitName := filepath.Base(strings.TrimSpace(r))
					actualUnits = append(actualUnits, unitName)
				}
			}
			// For .terragrunt-filters auto-detection test: the file contains "type=unit"
			// and we expect only "unit" in output, proving the file WAS automatically read
			assert.ElementsMatch(
				t,
				tc.expectedUnits,
				actualUnits,
				"Output mismatch for test case: %s",
				tc.name,
			)
		})
	}
}

// getJSONRunNames extracts unit names from records map for error messages
func getJSONRunNames(recordsByUnit map[string]*report.JSONRun) []string {
	names := make([]string, 0, len(recordsByUnit))
	for name := range recordsByUnit {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// TestFilterCompoundNegationDropsUnrelatedUnits pins that a filter whose negation comes first
// still restricts on its positive operand. A unit matching neither operand used to survive,
// because the whole query was treated as an exclusion of the negated operand alone.
func TestFilterCompoundNegationDropsUnrelatedUnits(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	for _, name := range []string{"foo", "bar", "baz"} {
		_ = createTestUnit(t, filepath.Join(tmpDir, name), "# "+name)
	}

	cmd := "terragrunt list --no-color --working-dir " + tmpDir + " --filter '!name=foo | name=bar'"
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Equal(t, []string{"bar"}, strings.Fields(stdout))
}

// TestFilterExcludeByDefault pins that a filter which only excludes still starts from every
// component, rather than emptying discovery out.
func TestFilterExcludeByDefault(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExcludeByDefault)
	rootPath := filepath.Join(tmpEnvPath, testFixtureExcludeByDefault)

	helpers.CleanupTerraformFolder(t, rootPath)

	cmd := "terragrunt list --no-color --working-dir " + rootPath + " --filter '!_stacks'"
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Equal(t, []string{"unit1"}, strings.Fields(stdout))
}

func TestFilterFlagWithMarkAsRead(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(testFixtureFilterMarkAsRead)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		filterQuery   string
		expectedUnits []string
		expectError   bool
	}{
		{
			name:          "filter by reading - exact match with unit path",
			filterQuery:   "reading=unit-normal/foo.txt",
			expectedUnits: []string{"unit-normal"},
			expectError:   false,
		},
		{
			name:          "filter by reading - wildcard matches foo.txt in multiple units",
			filterQuery:   "reading=*/foo.txt",
			expectedUnits: []string{"unit-normal", "unit-duplicate"},
			expectError:   false,
		},
		{
			name:          "filter by reading - bar.txt only in duplicate unit",
			filterQuery:   "reading=unit-duplicate/bar.txt",
			expectedUnits: []string{"unit-duplicate"},
			expectError:   false,
		},
		{
			name:          "filter by reading - wildcard *.txt matches all txt files",
			filterQuery:   "reading=*/*.txt",
			expectedUnits: []string{"unit-normal", "unit-duplicate"},
			expectError:   false,
		},
		{
			name:          "filter by reading - double wildcard",
			filterQuery:   "reading=**/foo.txt",
			expectedUnits: []string{"unit-normal", "unit-duplicate"},
			expectError:   false,
		},
		{
			name:          "filter by reading - non-matching file",
			filterQuery:   "reading=*/nonexistent.txt",
			expectedUnits: []string{},
			expectError:   false,
		},
		{
			name:          "filter by reading - empty string is parse error",
			filterQuery:   "reading=",
			expectedUnits: []string{},
			expectError:   true, // Empty value after = is a parse error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(
				t,
				err,
				"Unexpected error for filter query: %s\nstderr: %s",
				tc.filterQuery,
				stderr,
			)

			results := strings.Fields(stdout)
			assert.ElementsMatch(
				t,
				tc.expectedUnits,
				results,
				"Output mismatch for filter query: %s",
				tc.filterQuery,
			)
		})
	}
}

// TestFilterFlagWithMarkAsReadInIncludedConfig covers a relative path passed to mark_as_read from an
// included configuration. Such a path names a file relative to the include's own directory, the same
// way file() reads it, so a unit pulling in the include must be selected by a reading filter naming
// the file at that location rather than under the unit's directory.
func TestFilterFlagWithMarkAsReadInIncludedConfig(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(testFixtureFilterMarkAsReadInclude)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		filterQuery   string
		expectedUnits []string
	}{
		{
			name:          "relative path resolves against the included config's directory",
			filterQuery:   "reading=common/foo.txt",
			expectedUnits: []string{"unit-include"},
		},
		{
			name:          "the including unit's directory does not absorb the path",
			filterQuery:   "reading=unit-include/foo.txt",
			expectedUnits: []string{},
		},
		{
			name:          "a call outside an include still resolves against its own unit",
			filterQuery:   "reading=unit-plain/bar.txt",
			expectedUnits: []string{"unit-plain"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(
				t,
				err,
				"Unexpected error for filter query: %s\nstderr: %s",
				tc.filterQuery,
				stderr,
			)

			assert.ElementsMatch(
				t,
				tc.expectedUnits,
				strings.Fields(stdout),
				"Output mismatch for filter query: %s",
				tc.filterQuery,
			)
		})
	}
}

// TestFilterFlagWithGitFilterMarkGlobAsRead reproduces https://github.com/gruntwork-io/terragrunt/issues/6348:
// when a file matched by mark_glob_as_read is added or deleted across a Git diff, the unit that reads
// it via the glob must still be selected by a Git-based filter, even though the file lives outside the
// unit's own directory. Added files are matched against the "to" worktree; deleted files are matched
// against the "from" worktree, where the file still exists.
func TestFilterFlagWithGitFilterMarkGlobAsRead(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// Three units, each reading a sibling config directory via a glob. None of the watched files live
	// in the unit's own directory, so only the reading filter can connect a config change to its unit.
	units := map[string]string{
		"unit-reads-added":     `locals { config = mark_glob_as_read("../shared-added/*.yml") }`,
		"unit-reads-removed":   `locals { config = mark_glob_as_read("../shared-removed/*.yml") }`,
		"unit-reads-untouched": `locals { config = mark_glob_as_read("../shared-untouched/*.yml") }`,
	}

	for name, contents := range units {
		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(contents), 0644),
		)
	}

	// Baseline config files. shared-added is intentionally empty so the file lands as an addition later.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared-removed"), 0755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "shared-removed", "old.yml"), []byte("a: 1\n"), 0644),
	)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared-untouched"), 0755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "shared-untouched", "keep.yml"), []byte("b: 2\n"), 0644),
	)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	if b, err := runner.Config(t.Context(), "init.defaultBranch"); err != nil || b != "main" {
		require.NoError(t, runner.Checkout(t.Context(), "main", true))
	}

	require.NoError(t, runner.Checkout(t.Context(), "glob-read-test", true))

	// Add a file the glob matches, and delete an existing one the glob matched.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared-added"), 0755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "shared-added", "new.yml"), []byte("c: 3\n"), 0644),
	)
	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "shared-removed")))

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Add and remove glob-read config files"))

	helpers.CleanupTerraformFolder(t, tmpDir)

	cmd := "terragrunt find --no-color --working-dir " + tmpDir + " --filter '[HEAD~1...HEAD]'"
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stderr: %s", stderr)

	results := strings.Fields(stdout)
	assert.ElementsMatch(t, []string{"unit-reads-added", "unit-reads-removed"}, results,
		"units reading added or removed glob files should be selected; untouched unit should not")
}
