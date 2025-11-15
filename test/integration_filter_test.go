package test_test

import (
	"os"
	"path/filepath"
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
	testFixtureFilterBasic  = "fixtures/find/basic"
	testFixtureFilterDAG    = "fixtures/find/dag"
	testFixtureFilterList   = "fixtures/list/basic"
	testFixtureFilterSource = "fixtures/filter-source"
)

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

func TestFilterFlagWithFindGitFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

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

	t.Skip("This test won't pass until we fix the integration between discovery and runnerpool.")

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

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
		name          string
		filterQuery   string
		description   string
		expectedUnits []string
		excludedUnits []string
		expectError   bool
	}{
		{
			name:          "git filter discovers modified, created, and removed units and excludes untouched",
			filterQuery:   "[HEAD~1...HEAD]",
			expectedUnits: []string{"unit-to-be-created", "unit-to-be-modified", "unit-to-be-removed"},
			excludedUnits: []string{"unit-to-be-untouched"},
			expectError:   false,
			description:   "Git filter should discover units that were created, modified, or removed between commits, and exclude untouched units",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tmpDir)

			// Run terragrunt run --all --filter with git filter
			// Note: We use 'plan' command which should work even without terraform init
			cmd := "terragrunt run --all --no-color --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' -- plan"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				// For run commands, we expect some output even if terraform isn't fully initialized
				// The key is that the command should execute and process the filtered units
				// We check that the output contains references to the expected units
				if err != nil {
					// If there's an error, it might be because terraform isn't initialized
					// but we should still see that the filter worked (units were discovered)
					// Let's check if the error is about terraform init or similar
					if !strings.Contains(stderr, "terraform") && !strings.Contains(stderr, "tofu") {
						// Unexpected error
						require.NoError(t, err, "Unexpected error for filter query: %s\nstdout: %s\nstderr: %s", tc.filterQuery, stdout, stderr)
					}
				}

				// Verify that the expected units are mentioned in the output
				// The exact format may vary, but we should see references to these units
				output := stdout + stderr
				for _, expectedUnit := range tc.expectedUnits {
					// Check if the unit name appears in the output
					// This could be in paths, log messages, or error messages
					assert.Contains(t, output, expectedUnit,
						"Output should contain reference to unit '%s' for filter query: %s\nFull output:\n%s", expectedUnit, tc.filterQuery, output)
				}

				// Verify that excluded units are NOT in the output
				for _, excludedUnit := range tc.excludedUnits {
					assert.NotContains(t, output, excludedUnit,
						"Output should NOT contain reference to excluded unit '%s' for filter query: %s\nFull output:\n%s", excludedUnit, tc.filterQuery, output)
				}
			}
		})
	}
}

func TestFiltersFileFlag(t *testing.T) {
	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping filters file flag tests - TG_EXPERIMENT_MODE is enabled, and this test requires testing both")
	}

	t.Run("with experiment enabled", func(t *testing.T) {
		t.Setenv("TG_EXPERIMENT_MODE", "true")

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
				// Copy fixture to temporary directory
				tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterBasic)
				tmpDir := util.JoinPath(tmpEnvPath, testFixtureFilterBasic)

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
	})

	t.Run("experiment disabled scenarios", func(t *testing.T) {
		testCases := []struct {
			setupFile   func(t *testing.T, dir string) string
			name        string
			cmdFlags    string
			errorMsg    string
			expectError bool
		}{
			{
				name: "--filters-file flag returns error when experiment disabled",
				setupFile: func(t *testing.T, dir string) string {
					t.Helper()

					filterFile := filepath.Join(dir, "custom-filters.txt")
					err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
					require.NoError(t, err)
					return filterFile
				},
				cmdFlags:    "", // Will be set in test
				expectError: true,
				errorMsg:    "requires the 'filter-flag' experiment",
			},
			{
				name: ".terragrunt-filters file is silently ignored when experiment disabled",
				setupFile: func(t *testing.T, dir string) string {
					t.Helper()

					filterFile := filepath.Join(dir, ".terragrunt-filters")
					// Create filter file that would filter to only unit if read
					err := os.WriteFile(filterFile, []byte("type=unit\n"), 0644)
					require.NoError(t, err)
					return filterFile
				},
				cmdFlags:    "", // No flag, file should be ignored (not read)
				expectError: false,
				errorMsg:    "",
				// Note: expectedOutput verification happens in the test - should show all units
				// proving the filter file was NOT read
			},
			{
				name:        "--no-filters-file flag returns error when experiment disabled",
				setupFile:   nil,
				cmdFlags:    "--no-filters-file",
				expectError: true,
				errorMsg:    "requires the 'filter-flag' experiment",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Ensure experiment is disabled for this test case by unsetting the variable
				// This ensures the experiment is not enabled even if it was set in the parent environment
				require.NoError(t, os.Unsetenv("TG_EXPERIMENT_MODE"))
				// Copy fixture to temporary directory
				tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterBasic)
				tmpDir := util.JoinPath(tmpEnvPath, testFixtureFilterBasic)

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
				// Add --filters-file flag for custom filter files
				if filterFilePath != "" && filepath.Base(filterFilePath) != ".terragrunt-filters" {
					cmd += " --filters-file " + filterFilePath
				}

				stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

				if tc.expectError {
					require.Error(t, err, "Expected error for test case: %s", tc.name)

					if tc.errorMsg != "" {
						// Error message might be in stderr or in the error itself
						errStr := ""
						if err != nil {
							errStr = err.Error()
						}
						combinedOutput := stderr + stdout + errStr
						assert.Contains(t, combinedOutput, tc.errorMsg,
							"Expected error message containing '%s' in output for test case: %s\nstdout: %s\nstderr: %s\nerr: %v",
							tc.errorMsg, tc.name, stdout, stderr, err)
					}

					return
				}
				// For .terragrunt-filters ignored case, should succeed but not filter
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
				// When experiment is disabled, .terragrunt-filters file should be ignored
				// The file contains "type=unit" which would filter to only "unit" if read
				// But since it's ignored, we should see all units (stack and unit)
				// This proves the file was NOT automatically read
				expectedUnits := []string{"stack", "unit"}
				assert.ElementsMatch(t, expectedUnits, actualUnits,
					"When experiment is disabled, .terragrunt-filters should be ignored. "+
						"File contains 'type=unit' which would filter to only 'unit' if read, "+
						"but we see all units, proving the file was NOT read automatically.")
			})
		}
	})
}
