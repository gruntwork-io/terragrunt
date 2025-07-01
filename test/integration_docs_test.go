package test_test

import (
	"os"
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
