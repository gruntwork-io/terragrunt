package test_test

import (
	"os"
	"path/filepath"
	"strings"
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
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".txt") {
			txtFiles = append(txtFiles, filePath)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Len(t, txtFiles, 4)
}

// check if the stack directory is created and contains files
func validateStackDir(t *testing.T, path string) {
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
