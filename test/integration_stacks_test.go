package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/files"

	"github.com/gruntwork-io/terragrunt/config/hclparse"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureStacksBasic         = "fixtures/stacks/basic"
	testFixtureStacksLocals        = "fixtures/stacks/locals"
	testFixtureStacksRemote        = "fixtures/stacks/remote"
	testFixtureStacksInputs        = "fixtures/stacks/inputs"
	testFixtureStacksOutputs       = "fixtures/stacks/outputs"
	testFixtureStacksUnitValues    = "fixtures/stacks/unit-values"
	testFixtureStacksLocalsError   = "fixtures/stacks/errors/locals-error"
	testFixtureStacksUnitEmptyPath = "fixtures/stacks/errors/unit-empty-path"
	testFixtureStacksEmptyPath     = "fixtures/stacks/errors/stack-empty-path"
	testFixtureNestedStacks        = "fixtures/stacks/nested"
	testFixtureStackValues         = "fixtures/stacks/stack-values"
	testFixtureStackDependencies   = "fixtures/stacks/dependencies"
	testFixtureStackAbsolutePath   = "fixtures/stacks/absolute-path"
	testFixtureStackSourceMap      = "fixtures/stacks/source-map"
	testFixtureNoStack             = "fixtures/stacks/no-stack"
	testFixtureStackCycles         = "fixtures/stacks/errors/cycles"
	testFixtureNoStackNoDir        = "fixtures/stacks/no-stack-dir"
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

func TestNestedStacksGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
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

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --terragrunt-log-level debug --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "app1 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")
	assert.Contains(t, stderr, "app2 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")
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

	var result map[string]any
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

	var result map[string]any
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

	var result map[string]any
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

func TestStacksUnitValuesRunInApp1(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitValues)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// run apply in generated app1 directory
	app1Path := util.JoinPath(rootPath, ".terragrunt-stack", "app1")
	helpers.RunTerragrunt(t, "terragrunt apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+app1Path)

	// Verify the expected outcomes
	valuesPath := filepath.Join(app1Path, "terragrunt.values.hcl")
	assert.FileExists(t, valuesPath)

	// Verify the values file content
	content, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "deployment = \"app1\"")
}

func TestStacksUnitValuesOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitValues)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 2)
	// check if app1 and app2 are present in the result
	assert.Contains(t, result, "app1")
	assert.Contains(t, result, "app2")
}

func TestStacksUnitEmptyPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitEmptyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitEmptyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitEmptyPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	message := err.Error()
	// check for app1 and app2 empty path error
	assert.Contains(t, message, "unit 'app1_empty_path' has empty path")
	assert.Contains(t, message, "unit 'app2_empty_path' has empty path")
	assert.NotContains(t, message, "unit 'app3_not_empty_path' has empty path")
}

func TestStackStackEmptyPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEmptyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEmptyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksEmptyPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, "stack 'prod' has empty path")
}

func TestNestedStackOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 6)
	// check output contains stacks
	assert.Contains(t, result, "dev-api")
	assert.Contains(t, result, "dev-db")
	assert.Contains(t, result, "dev-web")

	assert.Contains(t, result, "prod-api")
	assert.Contains(t, result, "prod-db")
	assert.Contains(t, result, "prod-web")

}

func TestNestedStacksApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "data = \"web dev-web 1.0.0\"")
	assert.Contains(t, stdout, "data = \"api dev-api 1.0.0\"")
	assert.Contains(t, stdout, "data = \"db dev-db 1.0.0\"")

	assert.Contains(t, stdout, "data = \"web prod-web 1.0.0\"")
	assert.Contains(t, stdout, "data = \"api prod-api 1.0.0\"")
	assert.Contains(t, stdout, "data = \"db prod-db 1.0.0\"")
}

func TestStackValuesGeneration(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValues)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackValues)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// check that is generated terragrunt.values.hcl
	valuesPath := filepath.Join(path, "dev", "terragrunt.values.hcl")
	assert.FileExists(t, valuesPath)
}

func TestStackValuesApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValues)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackValues)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "project = \"dev-project\"")
	assert.Contains(t, stdout, "env = \"dev\"")
	assert.Contains(t, stdout, "data = \"dev-app-1\"")

	assert.Contains(t, stdout, "project = \"prod-project\"")
	assert.Contains(t, stdout, "env = \"prod\"")
	assert.Contains(t, stdout, "data = \"prod-app-1\"")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	devValuesPath := filepath.Join(path, "dev", "terragrunt.values.hcl")
	assert.FileExists(t, devValuesPath)

	prodValuesPath := filepath.Join(path, "prod", "terragrunt.values.hcl")
	assert.FileExists(t, prodValuesPath)
}

func TestStackValuesOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValues)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackValues)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")
	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 4)

	// check output contains stacks
	assert.Contains(t, result, "dev-app-1")
	assert.Contains(t, result, "dev-app-2")
	assert.Contains(t, result, "prod-app-1")
	assert.Contains(t, result, "prod-app-2")
}

func TestStacksGenerateParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --parallelism 10 --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStackApplyWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyWithDependencyParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --parallelism 10 --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyWithDependencyReducedParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --parallelism 1 --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyDestroyWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run destroy --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app-with-dependency")

	// check that the data.txt file was deleted
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.True(t, util.FileNotExists(dataPath))
}

func TestStackOutputWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	helpers.RunTerragrunt(t, "terragrunt stack run apply --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 4)

	assert.Contains(t, result, "app-with-dependency")
	assert.Contains(t, result, "app1")
	assert.Contains(t, result, "app2")
	assert.Contains(t, result, "app3")

	// check that result map under app-with-dependency contains result key with value "app1"
	if appWithDependency, ok := result["app-with-dependency"].(map[string]any); ok {
		assert.Contains(t, appWithDependency, "result")
		assert.Equal(t, "app1", appWithDependency["result"])
	} else {
		t.Errorf("Expected result[\"app-with-dependency\"] to be a map, but it was not.")
	}
}

func TestStackApplyStrictInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --experiment stacks stack run apply --queue-strict-include --queue-include-dir=./.terragrunt-stack/app1 --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app1")
	assert.NotContains(t, stderr, "Module ./.terragrunt-stack/app2")
	assert.NotContains(t, stderr, "Module ./.terragrunt-stack/app-with-dependency")

	// check that test file wasn't created
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.True(t, util.FileNotExists(dataPath))
}

func TestStacksSourceMap(t *testing.T) {
	t.Parallel()

	// prepare local path to do override of source url
	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	localTmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	localTmpTest := filepath.Join(localTmpEnvPath, "test", "fixtures")
	if err := os.MkdirAll(localTmpTest, 0755); err != nil {
		assert.NoError(t, err)
	}

	if err := files.CopyFolderContentsWithFilter(filepath.Join(localTmpEnvPath, "fixtures"), localTmpTest, func(path string) bool {
		return true
	}); err != nil {
		assert.NoError(t, err)
	}

	// prepare local environment with remote to use source map to replace
	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	// generate path with replacement of local source with local path
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --source-map git::https://github.com/gruntwork-io/terragrunt.git="+localTmpEnvPath+" --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Processing unit app1")
	assert.Contains(t, stderr, "Processing unit app2")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --terragrunt-log-level debug --experiment stacks --source-map git::https://github.com/gruntwork-io/terragrunt.git="+localTmpEnvPath+" --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// validate that the source map was used to replace the source
	assert.NotContains(t, stderr, "app1 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")
	assert.NotContains(t, stderr, "app2 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")

	assert.Contains(t, stderr, "Processing unit app1")
	assert.Contains(t, stderr, "Processing unit app2")
}

func TestStacksSourceMapModule(t *testing.T) {
	t.Parallel()
	// prepare local environment with remote to use source map to replace
	helpers.CleanupTerraformFolder(t, testFixtureStackSourceMap)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackSourceMap)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackSourceMap)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --source-map git::https://git-host.com/not-existing-repo.git="+tmpEnvPath+" --terragrunt-log-level debug --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.NotContains(t, stderr, "git-host.com/not-existing-repo.git")
	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --terragrunt-log-level debug --experiment stacks --source-map git::https://git-host.com/not-existing-repo.git="+tmpEnvPath+"  --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "git-host.com/not-existing-repo.git")
	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app1")
	assert.Contains(t, stderr, "Module ./.terragrunt-stack/app2")
}

func TestStacksGenerateAbsolutePath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackAbsolutePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackAbsolutePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackAbsolutePath)
	helpers.CreateGitRepo(t, rootPath)
	helpers.RunTerragrunt(t, "terragrunt stack generate --terragrunt-log-level debug --experiment stacks --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// check that apps directories are generated
	app3 := util.JoinPath(rootPath, ".terragrunt-stack", "app3")
	assert.DirExists(t, app3)

	app1 := util.JoinPath(rootPath, "app1")
	assert.DirExists(t, app1)

	app2 := util.JoinPath(rootPath, "app2")
	assert.DirExists(t, app2)

	app1 = util.JoinPath(rootPath, ".terragrunt-stack", "app1")
	assert.NoDirExists(t, app1)
}

func TestStacksGenerateNoStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStack)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNoStack)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)

	validateNoStackDirs(t, rootPath)
}

func TestStacksApplyNoStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStack)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNoStack)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "project")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --terragrunt-log-level debug --experiment stacks --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	validateNoStackDirs(t, rootPath)
}

func TestStacksCyclesErrors(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackCycles)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackCycles)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackCycles)
	helpers.CreateGitRepo(t, gitPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+gitPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cycle detected")
}

func TestStacksNoStackDirNoTerragruntStackDirectoryCreated(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStackNoDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStackNoDir)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureNoStackNoDir)

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment stacks --terragrunt-working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	// validate that the stack directory is not created
	assert.NoDirExists(t, path)
}

// validateNoStackDirs check if the directories outside of stack are created and contain test files
func validateNoStackDirs(t *testing.T, rootPath string) {
	t.Helper()
	stackConfig := util.JoinPath(rootPath, "stack-config")
	assert.DirExists(t, stackConfig)

	unitConfig := util.JoinPath(rootPath, "unit-config")
	assert.DirExists(t, unitConfig)

	configPath := util.JoinPath(stackConfig, "config.txt")
	assert.FileExists(t, configPath)

	configPath = util.JoinPath(unitConfig, "config.txt")
	assert.FileExists(t, configPath)

	secondStackUnitConfigDir := util.JoinPath(rootPath, ".terragrunt-stack", "dev", "second-stack-unit-config")
	secondStackUnitConfig := util.JoinPath(secondStackUnitConfigDir, "config.txt")

	assert.DirExists(t, secondStackUnitConfigDir)
	assert.FileExists(t, secondStackUnitConfig)
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
