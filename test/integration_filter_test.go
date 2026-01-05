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
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

const (
	testFixtureFilterBasic            = "fixtures/find/basic"
	testFixtureFilterDAG              = "fixtures/find/dag"
	testFixtureFilterList             = "fixtures/list/basic"
	testFixtureFilterSource           = "fixtures/filter-source"
	testFixtureMinimizeParsing        = "fixtures/filter/minimize-parsing"
	testFixtureMinimizeParsingDestroy = "fixtures/filter/minimize-parsing-destroy"
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
				assert.Equal(t, tc.expectedOutput, stdout, "Output mismatch for filter query: %s", tc.filterQuery)
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
				assert.JSONEq(t, tc.expectedOutput, stdout, "JSON output mismatch for filter query: %s", tc.filterQuery)
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

func TestFilterFlagWithFindGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	// Create three units initially
	unitToBeModifiedDir := filepath.Join(tmpDir, "unit-to-be-modified")
	unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
	unitToBeUntouchedDir := filepath.Join(tmpDir, "unit-to-be-untouched")

	err = os.MkdirAll(unitToBeModifiedDir, 0755)
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

	head, err := runner.GoOpenRepoHead()
	require.NoError(t, err)

	// If users don't have a default branch set, we'll make sure that the `main` branch exists
	b, err := runner.Config(t.Context(), "init.defaultBranch")
	if err != nil || b != "main" {
		err = runner.GoCheckout(&gogit.CheckoutOptions{
			Branch: plumbing.ReferenceName("refs/heads/main"),
			Create: true,
			Hash:   head.Hash(),
		})
		require.NoError(t, err)
	}

	// We'll checkout a new branch so that we can compare against main in the filter-affected flag test
	err = runner.GoCheckout(&gogit.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/filter-affected-test"),
		Create: true,
		Hash:   head.Hash(),
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
	err = os.MkdirAll(unitToBeCreatedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitToBeCreatedDir, "terragrunt.hcl"), []byte(`# Unit created`), 0644)
	require.NoError(t, err)

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
			name:          "standard git filter",
			filterQuery:   "[HEAD~1...HEAD]",
			expectedUnits: []string{"unit-to-be-created", "unit-to-be-modified", "unit-to-be-removed"},
			expectError:   false,
		},
		{
			name:                  "filter-affected flag",
			expectedUnits:         []string{"unit-to-be-created", "unit-to-be-modified", "unit-to-be-removed"},
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

func TestFilterFlagWithRunAllGitFilter(t *testing.T) {
	t.Parallel()

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

			tmpDir := helpers.TmpDirWOSymlinks(t)

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
			cmd := "terragrunt run --all --no-color --experiment-mode --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' --report-file " + helpers.ReportFile

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
				reportFilePath := filepath.Join(tmpDir, helpers.ReportFile)
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
					parts := strings.SplitSeq(fullPath, string(filepath.Separator))
					for part := range parts {
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

func TestFilterFlagWithRunAllGitFilterRemovedUnitDestroyFlag(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
	err = os.MkdirAll(unitToBeRemovedDir, 0755)
	require.NoError(t, err)

	terragruntHCL := `# Unit to be removed
terraform {
  source = "."
}
`
	err = os.WriteFile(filepath.Join(unitToBeRemovedDir, "terragrunt.hcl"), []byte(terragruntHCL), 0644)
	require.NoError(t, err)

	mainTF := `resource "null_resource" "test" {
  triggers = {
    test = "value"
  }
}
`
	err = os.WriteFile(filepath.Join(unitToBeRemovedDir, "main.tf"), []byte(mainTF), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit with unit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Apply the unit so that it shows up in state first.
	cmd := "terragrunt run --non-interactive --all --no-color --working-dir " + tmpDir + " -- apply"

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(
		t,
		stderr,
		"Unit unit-to-be-removed",
		"unit-to-be-removed should be discovered and run",
	)

	assert.Contains(
		t,
		stdout,
		"Apply complete! Resources: 1 added",
		"unit-to-be-removed should be applied",
	)

	err = os.RemoveAll(unitToBeRemovedDir)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Remove unit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	cmd = "terragrunt run --non-interactive --all --no-color --working-dir " + tmpDir +
		" --filter '[HEAD~1]' --filter-allow-destroy -- plan"

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, cmd)

	combinedOutput := stdout + stderr

	assert.Contains(
		t,
		combinedOutput,
		"unit-to-be-removed",
		"Removed unit should be discovered and processed",
	)

	// Check for destroy-related output. The message "No changes. No objects need to be destroyed"
	// is what Terraform outputs when plan -destroy is run but there's no state to destroy.
	// This is expected when using worktrees with local state (state is in original dir, not worktree).
	// The important thing is that the -destroy flag was passed, which we verify by checking for
	// this specific message that only appears with -destroy flag.
	hasDestroyFlag := strings.Contains(combinedOutput, "to destroy") ||
		strings.Contains(combinedOutput, "No objects need to be destroyed") ||
		strings.Contains(combinedOutput, "will be destroyed")

	assert.True(t, hasDestroyFlag,
		"Removed unit should be planned with -destroy flag. Output should contain 'to destroy', 'No objects need to be destroyed', or 'will be destroyed'. "+
			"Current output:\nstdout: %s\nstderr: %s", stdout, stderr)
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

			// Create a unit with the specified configuration
			unitDir := filepath.Join(tmpDir, "test-unit")
			unitHCLPath := createTestUnit(t, unitDir, tc.unitConfig)

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

			// Modify the unit to trigger Git filter detection
			err = os.WriteFile(unitHCLPath, []byte(tc.unitConfig+"\n# Modified"), 0644)
			require.NoError(t, err)

			// Commit the modification
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Modify unit", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Run terragrunt run --all --filter with git filter
			cmd := "terragrunt run --all --no-color --working-dir " + tmpDir + " --filter '[HEAD~1...HEAD]' --report-file " + helpers.ReportFile + " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			// Check for the warning in stderr
			// The warning message should contain this unique substring
			warningMessage := "do not have a remote_state configuration"
			hasWarning := strings.Contains(stderr, warningMessage) && strings.Contains(stderr, "Git-based filter expressions")

			if tc.expectWarning {
				assert.True(t, hasWarning, "Expected warning message in stderr. stderr: %s\nstdout: %s", stderr, stdout)
			} else {
				assert.False(t, hasWarning, "Did not expect warning message in stderr. stderr: %s\nstdout: %s", stderr, stdout)
			}

			// The command may fail due to the backend not being bootstrapped, but that's okay.
			// We're just checking for the warning
			_ = err
		})
	}
}

func TestFilterFlagWithExplicitStacksGitFilter(t *testing.T) {
	t.Parallel()

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
			name:               "git filter discovers units from modified, created, and removed stacks and excludes untouched",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: false,
			expectedUnits: []string{
				"unit-to-be-added",
				"unit-to-be-modified",
				"unit-to-be-created-1",
				"unit-to-be-created-2",
			},
			ignoredUnits: []string{
				"unit-to-be-untouched",
			},
			expectedExcluded: []string{
				"unit-to-be-removed-from-stack",
			},
			expectError: false,
			description: "Git filter should discover units from stacks that were created, modified, or removed between commits, and exclude untouched stacks. Units from removed stack should be excluded without --filter-allow-destroy",
		},
		{
			name:               "git filter with --filter-allow-destroy includes units from removed stack",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: true,
			expectedUnits: []string{
				"unit-to-be-added",
				"unit-to-be-modified",
				"unit-to-be-created-1",
				"unit-to-be-created-2",
				"unit-to-be-removed-from-stack",
			},
			ignoredUnits: []string{
				"unit-to-be-untouched",
			},
			expectedExcluded: []string{},
			expectError:      false,
			description:      "Git filter with --filter-allow-destroy should include units from removed stack for destroy operations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

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

			// Create a catalog of units that will be referenced by stacks
			legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
			err = os.MkdirAll(legacyUnitDir, 0755)
			require.NoError(t, err)
			_ = createTestUnit(t, legacyUnitDir, `# Legacy unit`)

			modernUnitDir := filepath.Join(tmpDir, "catalog", "units", "modern")
			err = os.MkdirAll(modernUnitDir, 0755)
			require.NoError(t, err)
			_ = createTestUnit(t, modernUnitDir, `# Modern unit`)

			// Create initial stacks
			stackToBeModifiedDir := filepath.Join(tmpDir, "live", "stack-to-be-modified")
			err = os.MkdirAll(stackToBeModifiedDir, 0755)
			require.NoError(t, err)

			stackToBeRemovedDir := filepath.Join(tmpDir, "live", "stack-to-be-removed")
			err = os.MkdirAll(stackToBeRemovedDir, 0755)
			require.NoError(t, err)

			stackToBeUntouchedDir := filepath.Join(tmpDir, "live", "stack-to-be-untouched")
			err = os.MkdirAll(stackToBeUntouchedDir, 0755)
			require.NoError(t, err)

			// Initial stack file contents
			initialStackContent := `unit "unit-to-be-modified" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-modified"
}

unit "unit-to-be-removed-from-stack" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-removed-from-stack"
}
`

			untouchedStackContent := `unit "unit-to-be-untouched" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-untouched"
}
`

			// Write initial stack files
			err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(initialStackContent), 0644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(stackToBeRemovedDir, "terragrunt.stack.hcl"), []byte(initialStackContent), 0644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(stackToBeUntouchedDir, "terragrunt.stack.hcl"), []byte(untouchedStackContent), 0644)
			require.NoError(t, err)

			// Initial commit
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Initial commit with stacks", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Modify the stack-to-be-modified: add a unit, modify a unit, remove a unit
			modifiedStackContent := `unit "unit-to-be-added" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-added"
}

unit "unit-to-be-modified" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-modified"
}
`
			err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(modifiedStackContent), 0644)
			require.NoError(t, err)

			// Remove the stack-to-be-removed
			err = os.RemoveAll(stackToBeRemovedDir)
			require.NoError(t, err)

			// Add a new stack
			stackToBeCreatedDir := filepath.Join(tmpDir, "live", "stack-to-be-created")
			err = os.MkdirAll(stackToBeCreatedDir, 0755)
			require.NoError(t, err)

			newStackContent := `unit "unit-to-be-created-1" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-created-1"
}

unit "unit-to-be-created-2" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-created-2"
}
`
			err = os.WriteFile(filepath.Join(stackToBeCreatedDir, "terragrunt.stack.hcl"), []byte(newStackContent), 0644)
			require.NoError(t, err)

			// Leave stack-to-be-untouched unchanged

			// Commit the changes
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Modify, create, and remove stacks", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Run terragrunt run --all --filter with git filter
			cmd := "terragrunt run --all --no-color --experiment-mode --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' --report-file " + helpers.ReportFile

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
				reportFilePath := filepath.Join(tmpDir, helpers.ReportFile)
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
					// Extract unit name from path
					// Paths might be like: /tmp/.../live/stack-to-be-modified/.terragrunt-stack/unit-to-be-added
					baseName := filepath.Base(fullPath)
					recordsByUnit[baseName] = record
					// Also store by full path for fallback
					recordsByUnit[fullPath] = record
					// Store by any part of the path that matches our unit pattern
					parts := strings.SplitSeq(fullPath, string(filepath.Separator))
					for part := range parts {
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
			if filterFilePath != "" && filepath.Base(filterFilePath) != ".terragrunt-filters" && !strings.Contains(tc.cmdFlags, "--filters-file") && !strings.Contains(tc.cmdFlags, "--no-filters-file") {
				cmd += " --filters-file " + filterFilePath
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for test case: %s", tc.name)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")

				return
			}

			require.NoError(t, err, "Unexpected error for test case: %s\nstdout: %s\nstderr: %s", tc.name, stdout, stderr)
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
			assert.ElementsMatch(t, tc.expectedUnits, actualUnits, "Output mismatch for test case: %s", tc.name)
		})
	}
}

func TestFilterFlagMinimizesParsing(t *testing.T) {
	t.Parallel()

	t.Run("single unit filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Run with filter targeting only target-unit
		// This will parse target-unit and its dependency (dependency-unit) for outputs,
		// but only target-unit will be run and appear in the report
		// The excluded units with land-mine configs should NOT be parsed
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath + " --filter './target-unit' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		// Verify that dependency-unit is still being parsed
		assert.Contains(t, stderr, "dependency-unit", "dependency-unit should be parsed")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		if util.FileExists(reportFilePath) {
			content, err := os.ReadFile(reportFilePath)
			require.NoError(t, err, "Should be able to read report file")

			var records []map[string]string

			err = json.Unmarshal(content, &records)
			require.NoError(t, err, "Should be able to parse report JSON")

			// Create a map of unit names to records for easier lookup
			recordsByUnit := make(map[string]map[string]string)

			for _, record := range records {
				fullPath := record["Name"]
				baseName := filepath.Base(fullPath)
				recordsByUnit[baseName] = record
				recordsByUnit[fullPath] = record
			}

			// Verify expected units are in the report
			found := false

			for name := range recordsByUnit {
				if strings.Contains(name, "target-unit") {
					found = true
					break
				}
			}

			require.True(t, found, "target-unit should be in report. Found units: %v", getUnitNames(recordsByUnit))

			// Verify land-mine units are NOT in the report
			for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
				found := false

				for name := range recordsByUnit {
					if strings.Contains(name, excludedUnit) {
						found = true
						break
					}
				}

				assert.False(t, found, "Excluded unit '%s' should NOT be in report", excludedUnit)
			}
		}
	})

	t.Run("multiple units filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Run with filter targeting both target-unit and dependency-unit (OR semantics)
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath + " --filter './target-unit' --filter './dependency-unit' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		if util.FileExists(reportFilePath) {
			content, err := os.ReadFile(reportFilePath)
			require.NoError(t, err, "Should be able to read report file")

			var records []map[string]string

			err = json.Unmarshal(content, &records)
			require.NoError(t, err, "Should be able to parse report JSON")

			// Create a map of unit names to records for easier lookup
			recordsByUnit := make(map[string]map[string]string)

			for _, record := range records {
				fullPath := record["Name"]
				baseName := filepath.Base(fullPath)
				recordsByUnit[baseName] = record
				recordsByUnit[fullPath] = record
			}

			// Verify expected units are in the report
			found := false

			for name := range recordsByUnit {
				if strings.Contains(name, "target-unit") {
					found = true
					break
				}
			}

			require.True(t, found, "target-unit should be in report. Found units: %v", getUnitNames(recordsByUnit))

			found = false

			for name := range recordsByUnit {
				if strings.Contains(name, "dependency-unit") {
					found = true
					break
				}
			}

			require.True(t, found, "dependency-unit should be in report. Found units: %v", getUnitNames(recordsByUnit))

			// Verify land-mine units are NOT in the report
			for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
				found := false

				for name := range recordsByUnit {
					if strings.Contains(name, excludedUnit) {
						found = true
						break
					}
				}

				assert.False(t, found, "Excluded unit '%s' should NOT be in report", excludedUnit)
			}
		}
	})

	t.Run("destroy without graph filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsingDestroy)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsingDestroy)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsingDestroy)

		// Run destroy with filter targeting only unit-a
		// This should only parse unit-a, NOT all units in the repository
		// The land-mine units should NOT be parsed
		cmd := "terragrunt run --all destroy --non-interactive --no-color --experiment-mode --working-dir " + rootPath + " --filter './unit-a' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "landmine-unit-1", "landmine-unit-1 should not be parsed during destroy")
		assert.NotContains(t, stderr, "landmine-unit-2", "landmine-unit-2 should not be parsed during destroy")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		if util.FileExists(reportFilePath) {
			content, err := os.ReadFile(reportFilePath)
			require.NoError(t, err, "Should be able to read report file")

			var records []map[string]string

			err = json.Unmarshal(content, &records)
			require.NoError(t, err, "Should be able to parse report JSON")

			// Create a map of unit names to records for easier lookup
			recordsByUnit := make(map[string]map[string]string)

			for _, record := range records {
				fullPath := record["Name"]
				baseName := filepath.Base(fullPath)
				recordsByUnit[baseName] = record
				recordsByUnit[fullPath] = record
			}

			// Verify expected unit is in the report
			found := false

			for name := range recordsByUnit {
				if strings.Contains(name, "unit-a") {
					found = true
					break
				}
			}

			require.True(t, found, "unit-a should be in report. Found units: %v", getUnitNames(recordsByUnit))

			// Verify land-mine units are NOT in the report
			for _, excludedUnit := range []string{"landmine-unit-1", "landmine-unit-2"} {
				found := false

				for name := range recordsByUnit {
					if strings.Contains(name, excludedUnit) {
						found = true
						break
					}
				}

				assert.False(t, found, "Excluded unit '%s' should NOT be in report", excludedUnit)
			}
		}
	})

	t.Run("destroy with graph filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsingDestroy)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsingDestroy)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsingDestroy)

		// Run destroy with graph filter targeting unit-a
		// Graph filters explicitly request dependency discovery, so this is expected behavior
		// The land-mine units should still NOT be parsed (they're not dependencies)
		cmd := "terragrunt run --all destroy --non-interactive --no-color --experiment-mode --working-dir " + rootPath + " --filter '{./unit-a}...' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		// Note: destroy might fail for other reasons (e.g., no state), but it shouldn't fail due to parsing land-mines
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "landmine-unit-1", "landmine-unit-1 should not be parsed during destroy with graph filter")
		assert.NotContains(t, stderr, "landmine-unit-2", "landmine-unit-2 should not be parsed during destroy with graph filter")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		if util.FileExists(reportFilePath) {
			content, err := os.ReadFile(reportFilePath)
			require.NoError(t, err, "Should be able to read report file")

			var records []map[string]string

			err = json.Unmarshal(content, &records)
			require.NoError(t, err, "Should be able to parse report JSON")

			// Create a map of unit names to records for easier lookup
			recordsByUnit := make(map[string]map[string]string)

			for _, record := range records {
				fullPath := record["Name"]
				baseName := filepath.Base(fullPath)
				recordsByUnit[baseName] = record
				recordsByUnit[fullPath] = record
			}

			// Verify expected unit is in the report
			found := false

			for name := range recordsByUnit {
				if strings.Contains(name, "unit-a") {
					found = true
					break
				}
			}

			require.True(t, found, "unit-a should be in report. Found units: %v", getUnitNames(recordsByUnit))

			// Verify land-mine units are NOT in the report
			for _, excludedUnit := range []string{"landmine-unit-1", "landmine-unit-2"} {
				found := false

				for name := range recordsByUnit {
					if strings.Contains(name, excludedUnit) {
						found = true
						break
					}
				}

				assert.False(t, found, "Excluded unit '%s' should NOT be in report", excludedUnit)
			}
		}
	})
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

// TestOutDirWithGitFilter verifies that --out-dir works correctly with git-based filters.
// This is a regression test for https://github.com/gruntwork-io/terragrunt/issues/5287
// The bug was that plan files were written to the temporary git worktree directory
// instead of the specified --out-dir path.
func TestOutDirWithGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

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

	// Create initial unit
	unitDir := filepath.Join(tmpDir, "unit-initial")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	// Create terragrunt.hcl
	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Initial unit`), 0644)
	require.NoError(t, err)

	// Create main.tf with a simple null resource
	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

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

	// Create a new unit (this will be detected by the git filter)
	newUnitDir := filepath.Join(tmpDir, "unit-new")
	err = os.MkdirAll(newUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(newUnitDir, "terragrunt.hcl"), []byte(`# New unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(newUnitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	// Commit the new unit
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Add new unit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Run terragrunt with --out-dir and git filter
	// The bug was that plan files went to /tmp/terragrunt-worktree-... instead of outDir
	cmd := "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " + tmpDir +
		" --out-dir " + outDir + " --filter '[HEAD~1...HEAD]' -- plan"

	helpers.RunTerragrunt(t, cmd)

	// Verify plan files are in outDir, NOT in a worktree path
	// The key assertion: plan files should be in outDir/unit-new/
	// NOT in /tmp/terragrunt-worktree-*/unit-new/
	files, err := filepath.Glob(filepath.Join(outDir, "**", "*.tfplan"))
	if err != nil {
		// Glob with ** doesn't work on all systems, try a walk
		files = []string{}
		_ = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
			require.NoError(t, err)

			if strings.HasSuffix(path, ".tfplan") {
				files = append(files, path)
			}

			return nil
		})
	}

	// Should have at least 1 plan file in outDir
	assert.NotEmpty(t, files, "Expected plan files in outDir %s", outDir)

	// None of the files should be in a worktree path
	for _, file := range files {
		assert.NotContains(t, file, "terragrunt-worktree",
			"Plan file %s should not be in a worktree directory", file)
		assert.True(t, strings.HasPrefix(file, outDir),
			"Plan file %s should be in outDir %s", file, outDir)
	}
}
