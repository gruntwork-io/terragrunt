package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureQuickStart       = "fixtures/docs/01-quick-start"
	testFixtureStacksLocalState = "fixtures/docs/03-stacks-with-local-state"
)

func TestDocsQuickStart(t *testing.T) {
	t.Parallel()

	t.Run("step-01", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-01", "foo")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy.")

		stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")

	})

	t.Run("step-01.1", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-01.1", "foo")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan -var content='Hello, Terragrunt!' --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve -var content='Hello, Terragrunt!' --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-02", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-02")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- plan -var content='Hello, Terragrunt!'")
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- apply -var content='Hello, Terragrunt!'")
		require.NoError(t, err)
	})

	t.Run("step-03", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-03")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- plan -var content='Hello, Terragrunt!'")
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- apply -var content='Hello, Terragrunt!'")
		require.NoError(t, err)
	})

	t.Run("step-04", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-04")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-05", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-05")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-06", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-06")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-07", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-07")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-07.1", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-07.1")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+rootPath)
		require.NoError(t, err)
	})
}

func TestStacksWithLocalState(t *testing.T) {
	t.Parallel()

	// Clean up the test fixture
	helpers.CleanupTerraformFolder(t, testFixtureStacksLocalState)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocalState)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocalState)
	livePath := util.JoinPath(rootPath, "live")
	localStatePath := util.JoinPath(rootPath, ".terragrunt-local-state")

	// Ensure local state directory doesn't exist initially
	require.NoError(t, os.RemoveAll(localStatePath))

	// Step 1: Generate the stack
	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+livePath)

	// Verify .terragrunt-stack directory was created
	stackPath := util.JoinPath(livePath, ".terragrunt-stack")
	require.DirExists(t, stackPath)

	// Verify individual units were generated
	fooPath := util.JoinPath(stackPath, "foo")
	barPath := util.JoinPath(stackPath, "bar")
	bazPath := util.JoinPath(stackPath, "baz")
	require.DirExists(t, fooPath)
	require.DirExists(t, barPath)
	require.DirExists(t, bazPath)

	// Step 2: Apply the stack to create state files
	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+livePath)

	// Verify local state files were created in .terragrunt-local-state
	// Note: path_relative_to_include() returns "live/.terragrunt-stack/foo" etc.
	fooStatePath := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "foo", "tofu.tfstate")
	barStatePath := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "bar", "tofu.tfstate")
	bazStatePath := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "baz", "tofu.tfstate")

	require.FileExists(t, fooStatePath)
	require.FileExists(t, barStatePath)
	require.FileExists(t, bazStatePath)

	// Verify state files contain actual state (not empty)
	fooStateContent, err := util.ReadFileAsString(fooStatePath)
	require.NoError(t, err)
	barStateContent, err := util.ReadFileAsString(barStatePath)
	require.NoError(t, err)
	bazStateContent, err := util.ReadFileAsString(bazStatePath)
	require.NoError(t, err)

	assert.Contains(t, fooStateContent, "null_resource")
	assert.Contains(t, barStateContent, "null_resource")
	assert.Contains(t, bazStateContent, "null_resource")

	// Step 3: Clean and regenerate the stack
	helpers.RunTerragrunt(t, "terragrunt stack clean --working-dir "+livePath)

	// Verify .terragrunt-stack directory was removed
	require.NoDirExists(t, stackPath)

	// Verify local state files still exist
	require.FileExists(t, fooStatePath)
	require.FileExists(t, barStatePath)
	require.FileExists(t, bazStatePath)

	// Regenerate the stack
	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+livePath)

	// Verify .terragrunt-stack directory was recreated
	require.DirExists(t, stackPath)
	require.DirExists(t, fooPath)
	require.DirExists(t, barPath)
	require.DirExists(t, bazPath)

	// Step 4: Verify that existing state is recognized after regeneration
	// Run plan to make sure it recognizes existing resources
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run plan --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	// The plan output should indicate no changes are needed since resources already exist
	assert.Contains(t, stdout, "No changes")

	// Step 5: Destroy resources to clean up
	helpers.RunTerragrunt(t, "terragrunt stack run destroy --non-interactive --working-dir "+livePath)

	// Verify state files still exist but are now empty/clean
	require.FileExists(t, fooStatePath)
	require.FileExists(t, barStatePath)
	require.FileExists(t, bazStatePath)
}

func TestStacksWithLocalStateFileStructure(t *testing.T) {
	t.Parallel()

	// Test that verifies the exact file structure created by the local state configuration
	helpers.CleanupTerraformFolder(t, testFixtureStacksLocalState)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocalState)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocalState)
	livePath := util.JoinPath(rootPath, "live")
	localStatePath := util.JoinPath(rootPath, ".terragrunt-local-state")

	// Ensure local state directory doesn't exist initially
	require.NoError(t, os.RemoveAll(localStatePath))

	// Generate and apply the stack
	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+livePath)
	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+livePath)

	// Test the exact structure of .terragrunt-local-state
	require.DirExists(t, localStatePath)

	// Check that each unit has its own subdirectory
	// Note: path structure reflects live/.terragrunt-stack/[unit]
	fooLocalStateDir := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "foo")
	barLocalStateDir := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "bar")
	bazLocalStateDir := util.JoinPath(localStatePath, "live", ".terragrunt-stack", "baz")

	require.DirExists(t, fooLocalStateDir)
	require.DirExists(t, barLocalStateDir)
	require.DirExists(t, bazLocalStateDir)

	// Check that state files are in the correct locations
	require.FileExists(t, util.JoinPath(fooLocalStateDir, "tofu.tfstate"))
	require.FileExists(t, util.JoinPath(barLocalStateDir, "tofu.tfstate"))
	require.FileExists(t, util.JoinPath(bazLocalStateDir, "tofu.tfstate"))

	// Since backend.tf is generated in the .terragrunt-cache directory during execution,
	// we verify the state files exist in the expected .terragrunt-local-state directory structure
	// This confirms that the backend configuration is working correctly

	// Verify the .terragrunt-local-state directory structure matches path_relative_to_include()
	liveStateDir := util.JoinPath(localStatePath, "live", ".terragrunt-stack")
	require.DirExists(t, liveStateDir)

	// Clean up
	helpers.RunTerragrunt(t, "terragrunt stack run destroy --non-interactive --working-dir "+livePath)
}

func TestFilterDocumentationExamples(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter documentation tests - TG_EXPERIMENT_MODE not enabled")
	}

	tmpDirRaw := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDirRaw)
	require.NoError(t, err)

	generateNameBasedFixture(t, tmpDir)
	generateAttributeBasedFixture(t, tmpDir)
	generatePathBasedFixture(t, tmpDir)
	generateNegationFixture(t, tmpDir)
	generateIntersectionFixture(t, tmpDir)
	generateReadingFixture(t, tmpDir)
	generateGraphBasedFixture(t, tmpDir)
	generateSourceBasedFixture(t, tmpDir)

	testCases := []struct {
		name           string
		fixtureDir     string
		filterQuery    string
		expectedOutput string
		extraFlags     string
	}{
		// Name-based filtering
		{
			name:           "name-based-exact-match",
			fixtureDir:     "name-based",
			filterQuery:    "app1",
			expectedOutput: "apps/app1\n",
		},
		{
			name:           "name-based-glob-pattern",
			fixtureDir:     "name-based",
			filterQuery:    "app*",
			expectedOutput: "apps/app1\napps/app2\n",
		},

		// Path-based filtering
		{
			name:           "path-based-relative-exact-match",
			fixtureDir:     "path-based",
			filterQuery:    "./envs/prod/apps/app1",
			expectedOutput: "envs/prod/apps/app1\n",
		},
		{
			name:           "path-based-relative-glob-pattern",
			fixtureDir:     "path-based",
			filterQuery:    "./envs/stage/**",
			expectedOutput: "envs/stage/apps/app1\nenvs/stage/apps/app2\n",
		},
		{
			name:           "path-based-absolute-exact-match",
			fixtureDir:     "path-based",
			filterQuery:    filepath.Join(tmpDir, "path-based", "root", "envs", "dev", "apps", "*"),
			expectedOutput: "envs/dev/apps/app1\nenvs/dev/apps/app2\n",
		},
		{
			name:           "path-based-braced-exact-match",
			fixtureDir:     "path-based",
			filterQuery:    "{./envs/prod/apps/app2}",
			expectedOutput: "envs/prod/apps/app2\n",
		},

		// Attribute-based filtering
		{
			name:           "attribute-type-unit",
			fixtureDir:     "attribute-based",
			filterQuery:    "type=unit",
			expectedOutput: "unit1\n",
		},
		{
			name:           "attribute-type-stack",
			fixtureDir:     "attribute-based",
			filterQuery:    "type=stack",
			expectedOutput: "stack1\n",
		},
		{
			name:           "attribute-based-external-false",
			fixtureDir:     "attribute-based",
			filterQuery:    "external=false",
			expectedOutput: "stack1\nunit1\n",
			extraFlags:     "--dependencies --external",
		},
		{
			name:           "attribute-based-external-true",
			fixtureDir:     "attribute-based",
			filterQuery:    "*... | external=true",
			expectedOutput: "../dependencies/dependency-of-app1\n",
			extraFlags:     "--dependencies --external",
		},
		{
			name:           "attribute-based-name-glob",
			fixtureDir:     "attribute-based",
			filterQuery:    "name=stack*",
			expectedOutput: "stack1\n",
		},

		// Negation
		{
			name:           "negation-by-name",
			fixtureDir:     "negation",
			filterQuery:    "!app1",
			expectedOutput: "envs/prod/apps/app2\nenvs/prod/stacks/stack1\nenvs/stage/apps/app2\nenvs/stage/stacks/stack1\n",
		},
		{
			name:           "negation-by-path",
			fixtureDir:     "negation",
			filterQuery:    "!./envs/prod/**",
			expectedOutput: "envs/stage/apps/app1\nenvs/stage/apps/app2\nenvs/stage/stacks/stack1\n",
		},
		{
			name:           "negation-by-attribute",
			fixtureDir:     "negation",
			filterQuery:    "!type=stack",
			expectedOutput: "envs/prod/apps/app1\nenvs/prod/apps/app2\nenvs/stage/apps/app1\nenvs/stage/apps/app2\n",
		},

		// Intersection
		{
			name:           "intersection-by-path-and-attribute",
			fixtureDir:     "intersection",
			filterQuery:    "./prod/** | type=unit",
			expectedOutput: "prod/units/unit1\nprod/units/unit2\n",
		},
		{
			name:           "intersection-by-path-and-negation",
			fixtureDir:     "intersection",
			filterQuery:    "./prod/** | !type=unit",
			expectedOutput: "prod/stacks/stack1\nprod/stacks/stack2\n",
		},
		{
			name:           "intersection-by-path-type-and-negation",
			fixtureDir:     "intersection",
			filterQuery:    "./dev/** | type=unit | !name=unit1",
			expectedOutput: "dev/units/unit2\n",
		},

		// Reading attribute filtering
		{
			name:           "reading-exact-file-match",
			fixtureDir:     "reading",
			filterQuery:    "reading=shared.hcl",
			expectedOutput: "apps/app1\napps/app2\n",
		},
		{
			name:           "reading-glob-pattern",
			fixtureDir:     "reading",
			filterQuery:    "reading=shared*",
			expectedOutput: "apps/app1\napps/app2\n",
		},
		{
			name:           "reading-nested-path",
			fixtureDir:     "reading",
			filterQuery:    "reading=common/vars.hcl",
			expectedOutput: "apps/app3\n",
		},
		{
			name:           "reading-negation",
			fixtureDir:     "reading",
			filterQuery:    "!reading=shared.hcl",
			expectedOutput: "apps/app3\nlibs/lib1\n",
		},
		{
			name:           "reading-intersection",
			fixtureDir:     "reading",
			filterQuery:    "./apps/** | reading=shared.hcl",
			expectedOutput: "apps/app1\napps/app2\n",
		},

		// Graph-based filtering
		{
			name:           "graph-dependency-traversal",
			fixtureDir:     "graph-based",
			filterQuery:    "service...",
			expectedOutput: "cache\ndb\nservice\nvpc\n",
		},
		{
			name:           "graph-dependent-traversal",
			fixtureDir:     "graph-based",
			filterQuery:    "...vpc",
			expectedOutput: "cache\ndb\nservice\nvpc\n",
		},
		{
			name:           "graph-both-directions",
			fixtureDir:     "graph-based",
			filterQuery:    "...db...",
			expectedOutput: "db\nservice\nvpc\n",
		},
		{
			name:           "graph-exclude-target",
			fixtureDir:     "graph-based",
			filterQuery:    "^service...",
			expectedOutput: "cache\ndb\nvpc\n",
		},
		{
			name:           "graph-with-path-filter",
			fixtureDir:     "graph-based",
			filterQuery:    "{./service}...",
			expectedOutput: "cache\ndb\nservice\nvpc\n",
		},
		{
			name:           "graph-with-attribute-filter",
			fixtureDir:     "graph-based",
			filterQuery:    "...name=vpc",
			expectedOutput: "cache\ndb\nservice\nvpc\n",
		},
		{
			name:           "graph-with-intersection",
			fixtureDir:     "graph-based",
			filterQuery:    "service... | !^db...",
			expectedOutput: "cache\ndb\nservice\n",
		},

		// Source-based filtering
		{
			name:           "source-exact-match-github",
			fixtureDir:     "source-based",
			filterQuery:    "source=github.com/acme/foo",
			expectedOutput: "github-acme-foo\n",
		},
		{
			name:           "source-exact-match-gitlab",
			fixtureDir:     "source-based",
			filterQuery:    "source=gitlab.com/example/baz",
			expectedOutput: "gitlab-example-baz\n",
		},
		{
			name:           "source-exact-match-local",
			fixtureDir:     "source-based",
			filterQuery:    "source=./module",
			expectedOutput: "local-module\n",
		},
		{
			name:           "source-glob-github-org",
			fixtureDir:     "source-based",
			filterQuery:    "source=*github.com**acme/*",
			expectedOutput: "github-acme-bar\ngithub-acme-foo\n",
		},
		{
			name:           "source-glob-github-ssh",
			fixtureDir:     "source-based",
			filterQuery:    "source=git::git@github.com:acme/**",
			expectedOutput: "github-acme-bar\n",
		},
		{
			name:           "source-glob-all-github",
			fixtureDir:     "source-based",
			filterQuery:    "source=**github.com**",
			expectedOutput: "github-acme-bar\ngithub-acme-foo\n",
		},
		{
			name:           "source-glob-gitlab",
			fixtureDir:     "source-based",
			filterQuery:    "source=gitlab.com/**",
			expectedOutput: "gitlab-example-baz\n",
		},
	}

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixturePath := filepath.Join(tmpDir, tc.fixtureDir)
			workingDir := filepath.Join(fixturePath, "root")

			// Run the find command with the filter
			command := fmt.Sprintf("terragrunt find --filter '%s' %s --working-dir %s", tc.filterQuery, tc.extraFlags, workingDir)
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, command)

			if err != nil {
				t.Logf("Command failed: %s", command)
				t.Logf("Error: %v", err)
				t.Logf("Output: %s", stdout)
			}

			require.NoError(t, err, "Command should succeed")
			assert.Equal(t, tc.expectedOutput, stdout, "Output should match expected result")
		})
	}
}

func TestFilterDocumentationExamplesWithUnion(t *testing.T) {
	t.Parallel()

	// Skip if experiment mode is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter documentation tests - TG_EXPERIMENT_MODE not enabled")
	}

	// Create temporary directory for dynamic fixtures
	tmpDirRaw := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDirRaw)
	require.NoError(t, err)

	// Generate fixtures for testing
	generateUnionFixture(t, tmpDir)

	// Test cases based on the documentation examples
	// Note: These tests demonstrate the intended functionality and will be updated
	// as the filter feature matures and becomes fully functional
	testCases := []struct {
		name           string
		fixtureDir     string
		filterQueries  []string
		expectedOutput string
	}{
		{
			name:           "union-by-two-names",
			fixtureDir:     "union",
			filterQueries:  []string{"unit1", "stack1"},
			expectedOutput: "dev/stack1\ndev/unit1\nenvs/prod/stack1\nenvs/prod/unit1\nenvs/stage/stack1\nenvs/stage/unit1\n",
		},
		{
			name:           "union-by-two-paths",
			fixtureDir:     "union",
			filterQueries:  []string{"./envs/prod/**", "./envs/stage/**"},
			expectedOutput: "envs/prod/stack1\nenvs/prod/stack2\nenvs/prod/unit1\nenvs/prod/unit2\nenvs/stage/stack1\nenvs/stage/stack2\nenvs/stage/unit1\nenvs/stage/unit2\n",
		},
		{
			name:           "union-by-name-and-negation",
			fixtureDir:     "union",
			filterQueries:  []string{"stack2", "!./envs/prod/**", "!./envs/stage/**"},
			expectedOutput: "dev/stack2\n",
		},
	}

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixturePath := filepath.Join(tmpDir, tc.fixtureDir)
			workingDir := filepath.Join(fixturePath, "root")

			// Run the find command with the filter
			var filterArgs []string
			for _, query := range tc.filterQueries {
				filterArgs = append(filterArgs, fmt.Sprintf("--filter %s", query))
			}
			command := fmt.Sprintf("terragrunt find %s --working-dir %s", strings.Join(filterArgs, " "), workingDir)
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, command)
			require.NoError(t, err, "Command should succeed")

			assert.Equal(t, tc.expectedOutput, stdout, "Output should match expected result")
		})
	}
}

// Helper functions to generate dynamic fixtures based on documentation examples

func generateNameBasedFixture(t *testing.T, baseDir string) {
	fixtureDir := filepath.Join(baseDir, "name-based", "root", "apps")
	require.NoError(t, os.MkdirAll(fixtureDir, 0755))

	// Create app1
	createTerragruntUnit(t, filepath.Join(fixtureDir, "app1"))
	// Create app2
	createTerragruntUnit(t, filepath.Join(fixtureDir, "app2"))
	// Create other (not matching the patterns)
	createTerragruntUnit(t, filepath.Join(fixtureDir, "other"))
}

func generateAttributeBasedFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "attribute-based", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create unit1
	createTerragruntUnitWithDependency(t, filepath.Join(rootDir, "unit1"), "../../dependencies/dependency-of-app1")
	// Create stack1
	createTerragruntStack(t, filepath.Join(rootDir, "stack1"))

	// Create external dependency
	depsDir := filepath.Join(baseDir, "attribute-based", "dependencies")
	require.NoError(t, os.MkdirAll(depsDir, 0755))
	createTerragruntUnit(t, filepath.Join(depsDir, "dependency-of-app1"))
}

func generatePathBasedFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "path-based", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create envs/prod/apps/app1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "apps", "app1"))
	// Create envs/prod/apps/app2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "apps", "app2"))
	// Create envs/stage/apps/app1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "apps", "app1"))
	// Create envs/stage/apps/app2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "apps", "app2"))
	// Create envs/dev/apps/app1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "dev", "apps", "app1"))
	// Create envs/dev/apps/app2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "dev", "apps", "app2"))
}

func generateNegationFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "negation", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create envs/prod/apps/app1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "apps", "app1"))
	// Create envs/prod/apps/app2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "apps", "app2"))
	// Create envs/prod/stacks/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "prod", "stacks", "stack1"))
	// Create envs/stage/apps/app1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "apps", "app1"))
	// Create envs/stage/apps/app2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "apps", "app2"))
	// Create envs/stage/stacks/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "stage", "stacks", "stack1"))
}

func generateIntersectionFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "intersection", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create prod/units/unit1
	createTerragruntUnit(t, filepath.Join(rootDir, "prod", "units", "unit1"))
	// Create prod/units/unit2
	createTerragruntUnit(t, filepath.Join(rootDir, "prod", "units", "unit2"))
	// Create prod/stacks/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "prod", "stacks", "stack1"))
	// Create prod/stacks/stack2
	createTerragruntStack(t, filepath.Join(rootDir, "prod", "stacks", "stack2"))
	// Create dev/units/unit1
	createTerragruntUnit(t, filepath.Join(rootDir, "dev", "units", "unit1"))
	// Create dev/units/unit2
	createTerragruntUnit(t, filepath.Join(rootDir, "dev", "units", "unit2"))
	// Create dev/stacks/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "dev", "stacks", "stack1"))
	// Create dev/stacks/stack2
	createTerragruntStack(t, filepath.Join(rootDir, "dev", "stacks", "stack2"))
}

func generateUnionFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "union", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create envs/prod/unit1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "unit1"))
	// Create envs/prod/unit2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "prod", "unit2"))
	// Create envs/prod/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "prod", "stack1"))
	// Create envs/prod/stack2
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "prod", "stack2"))
	// Create envs/stage/unit1
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "unit1"))
	// Create envs/stage/unit2
	createTerragruntUnit(t, filepath.Join(rootDir, "envs", "stage", "unit2"))
	// Create envs/stage/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "stage", "stack1"))
	// Create envs/stage/stack2
	createTerragruntStack(t, filepath.Join(rootDir, "envs", "stage", "stack2"))
	// Create dev/unit1
	createTerragruntUnit(t, filepath.Join(rootDir, "dev", "unit1"))
	// Create dev/unit2
	createTerragruntUnit(t, filepath.Join(rootDir, "dev", "unit2"))
	// Create dev/stack1
	createTerragruntStack(t, filepath.Join(rootDir, "dev", "stack1"))
	// Create dev/stack2
	createTerragruntStack(t, filepath.Join(rootDir, "dev", "stack2"))
}

func generateReadingFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "reading", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create shared configuration files
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "shared.hcl"), []byte(`
locals {
  common_value = "shared"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "shared.tfvars"), []byte(`
test_var = "value"
`), 0644))

	commonDir := filepath.Join(rootDir, "common")
	require.NoError(t, os.MkdirAll(commonDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commonDir, "vars.hcl"), []byte(`
locals {
  vpc_cidr = "10.0.0.0/16"
}
`), 0644))

	// Create apps/app1 - reads shared.hcl
	app1Dir := filepath.Join(rootDir, "apps", "app1")
	require.NoError(t, os.MkdirAll(app1Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(app1Dir, "terragrunt.hcl"), []byte(`
locals {
  shared = read_terragrunt_config("../../shared.hcl")
}

terraform {
  source = "."
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(app1Dir, "main.tf"), []byte(""), 0644))

	// Create apps/app2 - reads shared.hcl and shared.tfvars
	app2Dir := filepath.Join(rootDir, "apps", "app2")
	require.NoError(t, os.MkdirAll(app2Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(app2Dir, "terragrunt.hcl"), []byte(`
locals {
  shared = read_terragrunt_config("../../shared.hcl")
  vars = read_tfvars_file("../../shared.tfvars")
}

terraform {
  source = "."
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(app2Dir, "main.tf"), []byte(""), 0644))

	// Create apps/app3 - reads common/vars.hcl
	app3Dir := filepath.Join(rootDir, "apps", "app3")
	require.NoError(t, os.MkdirAll(app3Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(app3Dir, "terragrunt.hcl"), []byte(`
locals {
  common = read_terragrunt_config("../../common/vars.hcl")
}

terraform {
  source = "."
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(app3Dir, "main.tf"), []byte(""), 0644))

	// Create libs/lib1 - doesn't read any files
	lib1Dir := filepath.Join(rootDir, "libs", "lib1")
	createTerragruntUnit(t, lib1Dir)
}

func generateGraphBasedFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "graph-based", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create a dependency graph:
	// vpc (no dependencies)
	// db -> vpc
	// cache -> vpc
	// service -> db, cache

	// Create vpc (base dependency)
	vpcDir := filepath.Join(rootDir, "vpc")
	createTerragruntUnit(t, vpcDir)

	// Create db (depends on vpc)
	dbDir := filepath.Join(rootDir, "db")
	createTerragruntUnitWithDependency(t, dbDir, "../vpc")

	// Create cache (depends on vpc)
	cacheDir := filepath.Join(rootDir, "cache")
	createTerragruntUnitWithDependency(t, cacheDir, "../vpc")

	// Create service (depends on db and cache)
	serviceDir := filepath.Join(rootDir, "service")
	require.NoError(t, os.MkdirAll(serviceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(serviceDir, "terragrunt.hcl"), []byte(`terraform {
	source = "."
}

dependency "db" {
	config_path = "../db"
}

dependency "cache" {
	config_path = "../cache"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(serviceDir, "main.tf"), []byte(""), 0644))
}

func generateSourceBasedFixture(t *testing.T, baseDir string) {
	rootDir := filepath.Join(baseDir, "source-based", "root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create github-acme-foo with source github.com/acme/foo
	githubAcmeFooDir := filepath.Join(rootDir, "github-acme-foo")
	require.NoError(t, os.MkdirAll(githubAcmeFooDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(githubAcmeFooDir, "terragrunt.hcl"), []byte(`terraform {
  source = "github.com/acme/foo"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(githubAcmeFooDir, "main.tf"), []byte(""), 0644))

	// Create github-acme-bar with source git::git@github.com:acme/bar
	githubAcmeBarDir := filepath.Join(rootDir, "github-acme-bar")
	require.NoError(t, os.MkdirAll(githubAcmeBarDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(githubAcmeBarDir, "terragrunt.hcl"), []byte(`terraform {
  source = "git::git@github.com:acme/bar"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(githubAcmeBarDir, "main.tf"), []byte(""), 0644))

	// Create gitlab-example-baz with source gitlab.com/example/baz
	gitlabExampleBazDir := filepath.Join(rootDir, "gitlab-example-baz")
	require.NoError(t, os.MkdirAll(gitlabExampleBazDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(gitlabExampleBazDir, "terragrunt.hcl"), []byte(`terraform {
  source = "gitlab.com/example/baz"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(gitlabExampleBazDir, "main.tf"), []byte(""), 0644))

	// Create local-module with source ./module
	localModuleDir := filepath.Join(rootDir, "local-module")
	require.NoError(t, os.MkdirAll(localModuleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(localModuleDir, "terragrunt.hcl"), []byte(`terraform {
  source = "./module"
}
`), 0644))
	// Create the module directory with main.tf
	moduleDir := filepath.Join(localModuleDir, "module")
	require.NoError(t, os.MkdirAll(moduleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(""), 0644))

	// Create other-unit with source s3://bucket/module (for non-matching examples)
	otherUnitDir := filepath.Join(rootDir, "other-unit")
	require.NoError(t, os.MkdirAll(otherUnitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otherUnitDir, "terragrunt.hcl"), []byte(`terraform {
  source = "s3://bucket/module"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(otherUnitDir, "main.tf"), []byte(""), 0644))
}

// Helper functions to create Terragrunt configuration files

func createTerragruntUnit(t *testing.T, dir string) {
	require.NoError(t, os.MkdirAll(dir, 0755))
	// Create minimal terragrunt.hcl file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte("terraform {\n  source = \".\"\n}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0644))
}

func createTerragruntStack(t *testing.T, dir string) {
	require.NoError(t, os.MkdirAll(dir, 0755))
	// Create minimal terragrunt.stack.hcl file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.stack.hcl"), []byte("terraform {\n  source = \".\"\n}"), 0644))
}

func createTerragruntUnitWithDependency(t *testing.T, dir, dep string) {
	require.NoError(t, os.MkdirAll(dir, 0755))
	// Create minimal terragrunt.hcl file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(`terraform {
	source = "."
}

dependency "dep" {
	config_path = "`+dep+`"
}
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0644))
}
