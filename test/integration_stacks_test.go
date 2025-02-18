package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/config/hclparse"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureStacksBasic       = "fixtures/stacks/basic"
	testFixtureStacksLocals      = "fixtures/stacks/locals"
	testFixtureStacksLocalsError = "fixtures/stacks/errors/locals-error"
	testFixtureStacksRemote      = "fixtures/stacks/remote"
	testFixtureStacksInputs      = "fixtures/stacks/inputs"
	testFixtureStacksOutputs     = "fixtures/stacks/outputs"
	testFixtureStacksUnitValues  = "fixtures/stacks/unit-values"
	testFixtureStacksEmptyPath   = "fixtures/stacks/errors/empty-path"
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
	helpers.CreateGitRepo(t, tmpEnvPath)
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

func TestStacksApplyRemote(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed")
	assert.Contains(t, stdout, "local_file.file: Creation complete")
	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksApplyClean(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	path := util.JoinPath(rootPath, ".terragrunt-stack")
	// check that path exists
	assert.DirExists(t, path)

	helpers.RunTerragrunt(t, "terragrunt stack clean --experiment stacks --terragrunt-working-dir "+rootPath)
	// check that path don't exist
	assert.NoDirExists(t, path)
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

func TestStackOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stdout, "custom_value2 = \"value2\"")
	assert.Contains(t, stdout, "custom_value1 = \"value1\"")
	assert.Contains(t, stdout, "name      = \"name1\"")

	parser := hclparse.NewParser()
	hcl, diags := parser.ParseHCL([]byte(stdout), "test.hcl")
	assert.Nil(t, diags)
	attr, _ := hcl.Body.JustAttributes()
	assert.Len(t, attr, 4)
}

func TestStackOutputsIndex(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output project2_app2 --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
	assert.NotContains(t, stdout, "filtered_app1 = {")
	assert.Contains(t, stdout, "project2_app2 = {")

	parser := hclparse.NewParser()
	hcl, diags := parser.ParseHCL([]byte(stdout), "test.hcl")
	assert.Nil(t, diags)
	attr, _ := hcl.Body.JustAttributes()
	assert.Len(t, attr, 1)
}

func TestStackOutputsJson(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --format json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 4)
}

func TestStackOutputsJsonIndex(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output project2_app1 --format json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "project2_app1")
}

func TestStackOutputsRawError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --format raw --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.Error(t, err)
}

func TestStackOutputsRawIndex(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output filtered_app1.custom_value1 --format raw --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "value1")
	assert.NotContains(t, stdout, "filtered_app1 = {")
	assert.NotContains(t, stdout, "project2_app2 = {")
}

func TestStackOutputsRawFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -raw filtered_app2.data --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "app2")
	assert.NotContains(t, stdout, "project2_app1 = {")
	assert.NotContains(t, stdout, "project2_app2 = {")
}

func TestStackOutputsJsonFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 4)
}

func TestStacksUnitValues(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitValues)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "deployment = \"app1\"")
	assert.Contains(t, stdout, "deployment = \"app2\"")
	assert.Contains(t, stdout, "project = \"test-project\"")
	assert.Contains(t, stdout, "data = \"payload: app1-test-project\"")
	assert.Contains(t, stdout, "data = \"payload: app2-test-project\"")
}

func TestStacksUnitValuesOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitValues)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 2)
	// check if app1 and app2 are present in the result
	assert.Contains(t, result, "app1")
	assert.Contains(t, result, "app2")
}

func TestStacksEmptyPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEmptyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEmptyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksEmptyPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	message := err.Error()
	// check for app1 and app2 empty path error
	assert.Contains(t, message, "unit 'app1' has empty path")
	assert.Contains(t, message, "unit 'app2' has empty path")
	assert.NotContains(t, message, "unit 'app3' has empty path")
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
