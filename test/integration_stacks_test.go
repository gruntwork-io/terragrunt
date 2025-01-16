package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureStacksBasic       = "fixtures/stacks/basic"
	testFixtureStacksLocals      = "fixtures/stacks/locals"
	testFixtureStacksLocalsError = "fixtures/stacks/locals-error"
	testFixtureStacksRemote      = "fixtures/stacks/remote"
	testFixtureStacksInputs      = "fixtures/stacks/inputs"
)

func TestStacksGenerateBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocals)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocals)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocals)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)
}

func TestStacksGenerateLocalsError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocalsError)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocalsError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocalsError)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)
	require.Error(t, err)
}

func TestStacksGenerateRemote(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic)

	helpers.RunTerragrunt(t, "terragrunt --experiment stacks stack run apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// check that the stack was applied and .txt files got generated in the stack directory
	var txtFiles []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "test.txt" {
			txtFiles = append(txtFiles, filePath)
		}

		return nil
	})

	require.NoError(t, err)
	assert.Len(t, txtFiles, 4)
}

func TestStacksInputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs)

	helpers.RunTerragrunt(t, "terragrunt stack run plan --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksPlan(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run plan --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy")
	assert.Contains(t, stdout, "local_file.file will be created")
}

func TestStacksApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed")
	assert.Contains(t, stdout, "local_file.file: Creation complete")
}

func TestStacksDestroy(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run destroy --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Plan: 0 to add, 0 to change, 1 to destroy")
	assert.Contains(t, stdout, "local_file.file: Destroying...")
}

// check if the stack directory is created and contains files.
func validateStackDir(t *testing.T, path string) {
	t.Helper()
	assert.DirExists(t, path)

	// check that path is not empty directory
	entries, err := os.ReadDir(path)
	require.NoError(t, err, "Failed to read directory contents")

	hasSubdirectories := false
	for _, entry := range entries {
		if entry.IsDir() {
			hasSubdirectories = true

			break
		}
	}

	assert.True(t, hasSubdirectories, "The .terragrunt-stack directory should contain at least one subdirectory")
}
