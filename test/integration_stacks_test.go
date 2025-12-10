package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/files"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureStacksBasic                     = "fixtures/stacks/basic"
	testFixtureStacksLocals                    = "fixtures/stacks/locals"
	testFixtureStacksRemote                    = "fixtures/stacks/remote"
	testFixtureStacksInputs                    = "fixtures/stacks/inputs"
	testFixtureStacksOutputs                   = "fixtures/stacks/outputs"
	testFixtureStacksUnitValues                = "fixtures/stacks/unit-values"
	testFixtureStacksLocalsError               = "fixtures/stacks/errors/locals-error"
	testFixtureStacksUnitEmptyPath             = "fixtures/stacks/errors/unit-empty-path"
	testFixtureStacksEmptyPath                 = "fixtures/stacks/errors/stack-empty-path"
	testFixtureStackAbsolutePath               = "fixtures/stacks/errors/absolute-path"
	testFixtureStackRelativePathOutsideOfStack = "fixtures/stacks/errors/relative-path-outside-of-stack"
	testFixtureStackNotExist                   = "fixtures/stacks/errors/not-existing-path"
	testFixtureStackValidationUnitPath         = "fixtures/stacks/errors/validation-unit"
	testFixtureStackValidationStackPath        = "fixtures/stacks/errors/validation-stack"
	testFixtureStackIncorrectSource            = "fixtures/stacks/errors/incorrect-source"
	testFixtureNoStack                         = "fixtures/stacks/no-stack"
	testFixtureNestedStacks                    = "fixtures/stacks/nested"
	testFixtureStackValues                     = "fixtures/stacks/stack-values"
	testFixtureStackDependencies               = "fixtures/stacks/dependencies"
	testFixtureStackSourceMap                  = "fixtures/stacks/source-map"
	testFixtureStackCycles                     = "fixtures/stacks/errors/cycles"
	testFixtureNoStackNoDir                    = "fixtures/stacks/no-stack-dir"
	testFixtureMultipleStacks                  = "fixtures/stacks/multiple-stacks"
	testFixtureReadStack                       = "fixtures/stacks/read-stack"
	testFixtureStackSelfInclude                = "fixtures/stacks/self-include"
	testFixtureStackNestedOutputs              = "fixtures/stacks/nested-outputs"
	testFixtureStackNoValidation               = "fixtures/stacks/no-validation"
	testFixtureStackTerragruntDir              = "fixtures/stacks/terragrunt-dir"
	testFixtureStacksAllNoStackDir             = "fixtures/stacks/all-no-stack-dir"
	testFixtureStackNoDotTerragruntStackOutput = "fixtures/stacks/no-dot-terragrunt-stack-output"
	testFixtureStackFindInParentFolders        = "fixtures/stacks/find-in-parent-folders"
)

func TestStacksGenerateBasicWithQueueIncludeDirFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --queue-include-dir .terragrunt-stack/chicks/chick-2 --working-dir "+rootPath)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-1")
	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/father")
	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/mother")
	assert.Contains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-2")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateBasicWithFilterFlag(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --filter './.terragrunt-stack/chicks/chick-2' --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-1")
	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/father")
	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/mother")
	assert.Contains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-2")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateBasicWithQueueExcludeDirFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --queue-exclude-dir .terragrunt-stack/chicks/chick-2 --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-1")
	assert.Contains(t, stderr, "- Unit .terragrunt-stack/father")
	assert.Contains(t, stderr, "- Unit .terragrunt-stack/mother")
	assert.NotContains(t, stderr, "- Unit .terragrunt-stack/chicks/chick-2")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestNestedStacksGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, err)

	// Check that logs contain stack generation messages
	assert.Contains(t, stderr, "Generating stack prod from ./terragrunt.stack.hcl")
	assert.Contains(t, stderr, "Generating stack dev from ./terragrunt.stack.hcl")
	assert.Contains(t, stderr, "Generating unit prod-api from ./.terragrunt-stack/prod/terragrunt.stack.hcl")
	assert.Contains(t, stderr, "Generating unit dev-web from ./.terragrunt-stack/dev/terragrunt.stack.hcl")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocals)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocals)
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpEnvPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocals, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)
}

func TestStacksGenerateLocalsError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocalsError)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocalsError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocalsError)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)
}

func TestStacksGenerateRemote(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

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

func TestStacksNoGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// clean .terragrunt-stack contents
	entries, err := os.ReadDir(path)
	require.NoError(t, err)

	for _, entry := range entries {
		err = os.RemoveAll(filepath.Join(path, entry.Name()))
		require.NoError(t, err)
	}

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --no-stack-generate --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "No units discovered. Creating an empty runner.")
}

func TestStacksInputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run plan --non-interactive --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksPlan(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs, "live")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run plan --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy")
	assert.Contains(t, stdout, "local_file.file will be created")
}

func TestStacksApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs, "live")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed")
	assert.Contains(t, stdout, "local_file.file: Creation complete")
}

func TestStacksApplyRemote(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --log-level debug --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
	path := util.JoinPath(rootPath, ".terragrunt-stack")
	// check that path exists
	assert.DirExists(t, path)

	helpers.RunTerragrunt(t, "terragrunt stack clean --working-dir "+rootPath)
	// check that path don't exist
	assert.NoDirExists(t, path)
}

func TestStackCleanRecursively(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	live := util.JoinPath(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+live)
	require.NoError(t, err)

	liveV2 := util.JoinPath(gitPath, "live-v2")
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+liveV2)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack clean --working-dir "+gitPath)
	require.NoError(t, err)

	assert.NoDirExists(t, util.JoinPath(live, ".terragrunt-stack"))
	assert.NoDirExists(t, util.JoinPath(liveV2, ".terragrunt-stack"))

	assert.Contains(t, stderr, "Deleting stack directory: live/.terragrunt-stack")
	assert.Contains(t, stderr, "Deleting stack directory: live-v2/.terragrunt-stack")
}

func TestStacksDestroy(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksInputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run destroy --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Plan: 0 to add, 0 to change, 1 to destroy")
	assert.Contains(t, stdout, "local_file.file: Destroying...")
}

func TestStackOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --non-interactive --working-dir "+rootPath)

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

func TestStackOutputsRaw(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	// Using raw with no specific output key should return an error
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --format raw --non-interactive --working-dir "+rootPath)
	require.Error(t, err, "Should error when no specific output key is provided with --format raw")
	assert.Contains(t, err.Error(), "requires a single output value")

	// With a specific key, it should work for simple values
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output filtered_app1.custom_value1 --format raw --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Equal(t, "value1", strings.TrimSpace(stdout), "Raw output should print only the value without quotes")

	// Complex values should return an error
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output filtered_app1.complex --format raw --non-interactive --working-dir "+rootPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unsupported value for raw output")
}

func TestStackOutputsIndex(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output project2_app2 --non-interactive --working-dir "+rootPath)

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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output --format json --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output project2_app1 --format json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "project2_app1")
}

func TestStackOutputsRawIndex(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output filtered_app1.custom_value1 --format raw --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "value1")
	assert.NotContains(t, stdout, "filtered_app1 = {")
	assert.NotContains(t, stdout, "project2_app2 = {")
}

func TestStackOutputsRawFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -raw filtered_app2.data --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "app2")
	assert.NotContains(t, stdout, "project2_app1 = {")
	assert.NotContains(t, stdout, "project2_app2 = {")
}

func TestStackOutputsJsonFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksOutputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksOutputs, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues, "live")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	// run apply in generated app1 directory
	app1Path := util.JoinPath(rootPath, ".terragrunt-stack", "app1")
	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --working-dir "+app1Path)

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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitValues, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksUnitEmptyPath, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
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

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, "stack 'prod' has empty path")
}

func TestNestedStackOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")
	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output -json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	assert.Contains(t, result, "dev")
	assert.Contains(t, result, "prod")

	// Check dev outputs
	devOutputs := result["dev"].(map[string]any)
	assert.Contains(t, devOutputs, "dev-api")
	assert.Contains(t, devOutputs, "dev-db")
	assert.Contains(t, devOutputs, "dev-web")

	assert.Equal(t, "api dev-api 1.0.0", devOutputs["dev-api"].(map[string]any)["data"])
	assert.Equal(t, "db dev-db 1.0.0", devOutputs["dev-db"].(map[string]any)["data"])
	assert.Equal(t, "web dev-web 1.0.0", devOutputs["dev-web"].(map[string]any)["data"])

	// Check prod outputs
	prodOutputs := result["prod"].(map[string]any)
	assert.Contains(t, prodOutputs, "prod-api")
	assert.Contains(t, prodOutputs, "prod-db")
	assert.Contains(t, prodOutputs, "prod-web")

	assert.Equal(t, "api prod-api 1.0.0", prodOutputs["prod-api"].(map[string]any)["data"])
	assert.Equal(t, "db prod-db 1.0.0", prodOutputs["prod-db"].(map[string]any)["data"])
	assert.Equal(t, "web prod-web 1.0.0", prodOutputs["prod-web"].(map[string]any)["data"])
}

func TestNestedStacksApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacks)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
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

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")
	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

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

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
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

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")
	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	var result map[string]map[string]map[string]string

	err = json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err)

	// Check the structure of the JSON
	assert.Contains(t, result, "dev")
	assert.Contains(t, result, "prod")

	// Check dev-app-1
	devApp1 := result["dev"]["dev-app-1"]
	assert.Equal(t, "dev-project dev dev-app-1", devApp1["config"])
	assert.Equal(t, "dev-app-1", devApp1["data"])
	assert.Equal(t, "dev", devApp1["env"])
	assert.Equal(t, "dev-project", devApp1["project"])

	// Check dev-app-2
	devApp2 := result["dev"]["dev-app-2"]
	assert.Equal(t, "dev-project dev dev-app-2", devApp2["config"])
	assert.Equal(t, "dev-app-2", devApp2["data"])
	assert.Equal(t, "dev", devApp2["env"])
	assert.Equal(t, "dev-project", devApp2["project"])

	// Check prod-app-1
	prodApp1 := result["prod"]["prod-app-1"]
	assert.Equal(t, "prod-project prod prod-app-1", prodApp1["config"])
	assert.Equal(t, "prod-app-1", prodApp1["data"])
	assert.Equal(t, "prod", prodApp1["env"])
	assert.Equal(t, "prod-project", prodApp1["project"])

	// Check prod-app-2
	prodApp2 := result["prod"]["prod-app-2"]
	assert.Equal(t, "prod-project prod prod-app-2", prodApp2["config"])
	assert.Equal(t, "prod-app-2", prodApp2["data"])
	assert.Equal(t, "prod", prodApp2["env"])
	assert.Equal(t, "prod-project", prodApp2["project"])
}

func TestStacksGenerateParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --parallelism 10 --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStackApplyWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyWithDependencyParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --parallelism 10 --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyWithDependencyReducedParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --parallelism 1 --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

	// check that test
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.FileExists(t, dataPath)
}

func TestStackApplyDestroyWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run destroy --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

	// check that the data.txt file was deleted
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.True(t, util.FileNotExists(dataPath))
}

func TestStackOutputWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack output -json --non-interactive --working-dir "+rootPath)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --non-interactive --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run apply --queue-strict-include --queue-include-dir=./.terragrunt-stack/app1 --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/app1")
	assert.NotContains(t, stderr, "Unit .terragrunt-stack/app2")
	assert.NotContains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

	// check that test file wasn't created
	dataPath := util.JoinPath(rootPath, ".terragrunt-stack", "app-with-dependency", "data.txt")
	assert.True(t, util.FileNotExists(dataPath))
}

func TestStackApplyStrictIncludeWithFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDependencies, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --non-interactive --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run apply --filter ./.terragrunt-stack/app1 --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/app1")
	assert.NotContains(t, stderr, "Unit .terragrunt-stack/app2")
	assert.NotContains(t, stderr, "Unit .terragrunt-stack/app-with-dependency")

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
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --source-map git::https://github.com/gruntwork-io/terragrunt.git="+localTmpEnvPath+" --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Generating unit app1")
	assert.Contains(t, stderr, "Generating unit app2")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --log-level debug --source-map git::https://github.com/gruntwork-io/terragrunt.git="+localTmpEnvPath+" --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	// validate that the source map was used to replace the source
	assert.NotContains(t, stderr, "app1 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")
	assert.NotContains(t, stderr, "app2 (git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=main&depth=1)")

	assert.Contains(t, stderr, "Running ./.terragrunt-stack/app1")
	assert.Contains(t, stderr, "Running ./.terragrunt-stack/app2")
}

func TestStacksSourceMapModule(t *testing.T) {
	t.Parallel()
	// prepare local environment with remote to use source map to replace
	helpers.CleanupTerraformFolder(t, testFixtureStackSourceMap)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackSourceMap)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackSourceMap, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t, "terragrunt stack generate --source-map git::https://git-host.com/not-existing-repo.git="+tmpEnvPath+" --log-level debug --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.NotContains(t, stderr, "git-host.com/not-existing-repo.git")

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack run apply --log-level debug --source-map git::https://git-host.com/not-existing-repo.git="+tmpEnvPath+"  --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "git-host.com/not-existing-repo.git")
	assert.Contains(t, stderr, "Unit .terragrunt-stack/app1")
	assert.Contains(t, stderr, "Unit .terragrunt-stack/app2")
}

func TestStacksGenerateAbsolutePathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackAbsolutePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackAbsolutePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackAbsolutePath, "live")

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)
}

func TestStacksGenerateIncorrectSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackIncorrectSource)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackIncorrectSource)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackIncorrectSource, "live")

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to fetch unit api")
}

func TestStacksGenerateRelativePathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackRelativePathOutsideOfStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackRelativePathOutsideOfStack)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackRelativePathOutsideOfStack, "live")

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)

	assert.Contains(t, err.Error(), "app1 destination path")
	assert.Contains(t, err.Error(), "is outside of the stack directory")
}

func TestStacksGenerateNoStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStack)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNoStack)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	validateNoStackDirs(t, rootPath)
}

func TestStacksApplyNoStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStack)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNoStack)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --log-level debug --non-interactive --working-dir "+rootPath)

	validateNoStackDirs(t, rootPath)
}

func TestStacksCyclesErrors(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackCycles)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackCycles)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackCycles)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)

	// On macOS, the error that the filename is too long happens before cycles are detected.
	if !strings.Contains(err.Error(), "Cycle detected") {
		assert.Contains(t, err.Error(), "file name too long")

		return
	}

	assert.Contains(t, err.Error(), "Cycle detected")
}

func TestStacksNoStackDirDirectoryCreated(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStackNoDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStackNoDir)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureNoStackNoDir, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	// validate that the stack directory is not created
	assert.NoDirExists(t, path)
}

func TestStacksGeneratePrintWarning(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	assert.Contains(t, stderr, "No stack files found")
	require.NoError(t, err)
}

func TestStacksNotExistingPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackNotExist)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackNotExist)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackNotExist)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)
}

func TestStacksGenerateMultipleStacks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMultipleStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMultipleStacks)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureMultipleStacks)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	devStack := util.JoinPath(rootPath, "dev", ".terragrunt-stack")
	validateStackDir(t, devStack)

	liveStack := util.JoinPath(rootPath, "live", ".terragrunt-stack")
	validateStackDir(t, liveStack)
}

func TestStacksReadFiles(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReadStack)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureReadStack)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --log-level debug --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "stack_local_project = \"test-project\"")
	assert.Contains(t, stdout, "unit_value_version  = \"6.6.6\"")

	parser := hclparse.NewParser()
	hcl, diags := parser.ParseHCL([]byte(stdout), "test.hcl")
	assert.Nil(t, diags)

	attr, _ := hcl.Body.JustAttributes()
	assert.Len(t, attr, 3)

	// fetch for dev-app-2 output
	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output dev --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	hcl, diags = parser.ParseHCL([]byte(stdout), "dev.hcl")
	require.Nil(t, diags, diags.Error())

	topLevelAttrs, _ := hcl.Body.JustAttributes()
	assert.Len(t, topLevelAttrs, 1, "Expected one top-level attribute (dev)")

	devAttr, exists := topLevelAttrs["dev"]
	assert.True(t, exists, "dev block should exist")

	if exists {
		devObjVal, diags := devAttr.Expr.Value(nil)
		assert.Nil(t, diags)

		if devObjVal.Type().IsObjectType() {
			devApp2Attr := devObjVal.GetAttr("dev-app-2")
			assert.False(t, devApp2Attr.IsNull(), "dev-app-2 block should exist in dev")

			if !devApp2Attr.IsNull() {
				objVal := devApp2Attr
				assert.False(t, objVal.IsNull(), "dev-app-2 block should exist in dev")

				if !objVal.IsNull() {
					expectedValues := map[string]string{
						"config":              "dev-project dev dev-app-2",
						"data":                "dev-app-2",
						"env":                 "dev",
						"project":             "dev-project",
						"stack_local_project": "test-project",
						"stack_value_env":     "dev",
						"unit_name":           "test_app",
						"unit_source":         "../units/app",
						"unit_value_version":  "6.6.6",
					}

					if objVal.Type().IsObjectType() {
						for field, expectedValue := range expectedValues {
							attrVal := objVal.GetAttr(field)
							assert.False(t, attrVal.IsNull(), "Field %s should exist in output", field)

							if !attrVal.IsNull() {
								assert.Equal(
									t,
									expectedValue,
									attrVal.AsString(),
									"Field %s should have value %s",
									field,
									expectedValue,
								)
							}
						}

						stackSource := objVal.GetAttr("stack_source")
						assert.False(t, stackSource.IsNull(), "Field stack_source should exist in output")

						if !stackSource.IsNull() {
							assert.Contains(t, stackSource.AsString(), "/fixtures/stacks/read-stack/stacks/dev")
						}

						// Verify expected fields count (including stack_source)
						valueMap := objVal.AsValueMap()
						assert.Len(
							t,
							valueMap,
							len(expectedValues)+1,
							"Expected %d fields in dev-app-2",
							len(expectedValues)+1,
						)
					} else {
						t.Fatalf("Expected dev-app-2 to be an object type, got %s", objVal.Type().FriendlyName())
					}
				}
			}
		} else {
			t.Fatalf("Expected dev to be an object type, got %s", devObjVal.Type().FriendlyName())
		}
	}
}

func TestStackUnitValidation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValidationUnitPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValidationUnitPath)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackValidationUnitPath)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-stack-validate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	liveStack := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, liveStack)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Validation failed for unit v1")
	assert.Contains(t, err.Error(), "expected unit to generate with terragrunt.hcl file at root of generated directory")
}

func TestStackValidation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValidationStackPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValidationStackPath)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackValidationStackPath)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-stack-validate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	liveStack := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, liveStack)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Validation failed for stack stack-v1")
	assert.Contains(
		t,
		err.Error(),
		"expected stack to generate with terragrunt.stack.hcl file at root of generated directory",
	)
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

func TestStacksSelfInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackSelfInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackSelfInclude)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackSelfInclude, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// validate that subsequent runs don't fail
	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)
}

func TestStackNestedOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackNestedOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackNestedOutputs)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackNestedOutputs)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack run apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// Parse the HCL output
	parser := hclparse.NewParser()
	hclFile, diags := parser.ParseHCL([]byte(stdout), "test.hcl")
	require.False(t, diags.HasErrors(), "Failed to parse HCL: %s", diags.Error())

	require.Nil(t, diags)
	require.NotNil(t, hclFile)

	topLevelAttrs, _ := hclFile.Body.JustAttributes()
	_, app1Exists := topLevelAttrs["app_1"]
	assert.True(t, app1Exists, "app_1 block should exist")

	_, app2Exists := topLevelAttrs["app_2"]
	assert.True(t, app2Exists, "app_2 block should exist")

	_, stackV2Exists := topLevelAttrs["stack_v2"]
	assert.True(t, stackV2Exists, "stack_v2 block should exist")
}

func TestStacksNoValidation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackNoValidation)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackNoValidation)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackNoValidation, "live")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run plan --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit .terragrunt-stack/stack1/stack1/.terragrunt-stack/unit2/app1/code")
	assert.Contains(t, stderr, "Unit .terragrunt-stack/unit1/app1/code")

	assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy")
	assert.Contains(t, stdout, "local_file.file will be created")
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

func TestStackTerragruntDir(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackTerragruntDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackTerragruntDir)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureStackTerragruntDir)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := util.JoinPath(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-stack-validate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	out, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --all --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.Contains(t, out, `terragrunt_dir = "./tennant_1"`)
}

func TestStackRunAllNoStackDir(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksAllNoStackDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksAllNoStackDir)

	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksAllNoStackDir, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// Verify that no .terragrunt-stack directory was created since all units have no_dot_terragrunt_stack = true
	stackDir := util.JoinPath(rootPath, ".terragrunt-stack")
	stackDirExists := util.FileExists(stackDir)
	t.Logf("Stack directory exists: %v", stackDirExists)

	// Verify that units were generated in the same directory as terragrunt.stack.hcl
	expectedUnits := []string{"foo", "bar"}
	for _, unit := range expectedUnits {
		unitPath := util.JoinPath(rootPath, unit)
		assert.True(t, util.FileExists(unitPath), "Expected unit %s to exist in root directory", unit)
		assert.True(t, util.FileExists(
			util.JoinPath(unitPath, "terragrunt.hcl"),
		), "Expected terragrunt.hcl to exist in unit %s", unit)
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run plan --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err, "Expected stack run to succeed when all units have no_dot_terragrunt_stack = true")

	assert.Contains(t, stdout, "Changes to Outputs:")
	assert.Contains(t, stdout, "+ test = \"value\"")
}

func TestStackOutputWithNoDotTerragruntStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackNoDotTerragruntStackOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackNoDotTerragruntStackOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackNoDotTerragruntStackOutput, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	unitPath := util.JoinPath(rootPath, "app1")
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+unitPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "name = \"app1\"")
}

func TestStackFindInParentFolders(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackFindInParentFolders)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackFindInParentFolders)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackFindInParentFolders)
	stackPath := util.JoinPath(rootPath, "live", "stack")

	// Run stack with --queue-exclude-dir to exclude units source directory
	// This tests that source templates are skipped during parsing
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run plan --queue-exclude-dir '**/units/**' --working-dir "+stackPath,
	)
	require.NoError(t, err)

	// Verify generated unit runs
	assert.Contains(t, stderr, "- Unit .terragrunt-stack/foo")

	// Verify source template is excluded
	assert.NotContains(t, stderr, "- Unit units/foo")
}

func TestStackGenerateWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNestedStacks)
	liveDir := filepath.Join(rootPath, "live")

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir,
	)

	stackDir := filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir := filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.DirExists(t, devDir)

	prodDir := filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.DirExists(t, prodDir)

	require.NoError(t, os.RemoveAll(stackDir))

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir+" --filter 'live | type=stack' --filter 'dev | type=stack'",
	)

	stackDir = filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir = filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.DirExists(t, devDir)

	prodDir = filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.NoDirExists(t, prodDir)

	require.NoError(t, os.RemoveAll(stackDir))

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir+" --filter 'live | type=stack' --filter 'prod | type=stack'",
	)

	stackDir = filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir = filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.NoDirExists(t, devDir)

	prodDir = filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.DirExists(t, prodDir)
}

func TestStackGenerationWithNestedTopologyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	setupNestedStackFixture(t, tmpDir)

	liveDir := filepath.Join(tmpDir, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+liveDir)
	require.NoError(t, err)

	stackDir := filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	foundFiles := findStackFiles(t, liveDir)
	require.NotEmpty(t, foundFiles, "Expected to find generated stack files")

	l := logger.CreateLogger()
	topology := generate.BuildStackTopology(l, foundFiles, liveDir)
	require.NotEmpty(t, topology, "Expected non-empty topology")

	levelCounts := make(map[int]int)
	for _, node := range topology {
		levelCounts[node.Level]++
	}

	t.Logf("Topology levels found: %v", levelCounts)

	assert.Len(t, levelCounts, 3, "Expected levels in nested topology")

	assert.Equal(t, 1, levelCounts[0], "Level 0 should have exactly 1 stack file")
	assert.Equal(t, 3, levelCounts[1], "Level 1 should have exactly 3 stack files")
	assert.Equal(t, 9, levelCounts[2], "Level 2 should have exactly 9 stack files")

	verifyGeneratedUnits(t, stackDir)

	// Run one more time just to be sure things don't break when running in a dirty directory
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+liveDir)
	require.NoError(t, err)
}

// setupNestedStackFixture creates a test fixture similar to testing-nested-stacks
func setupNestedStackFixture(t *testing.T, tmpDir string) {
	t.Helper()

	liveDir := filepath.Join(tmpDir, "live")
	stacksDir := filepath.Join(tmpDir, "stacks")
	unitsDir := filepath.Join(tmpDir, "units")

	require.NoError(t, os.MkdirAll(liveDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "foo"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "final"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(unitsDir, "final"), 0755))

	liveStackConfig := `stack "foo" {
  source = "../stacks/foo"
  path   = "foo"
}

stack "foo2" {
  source = "../stacks/foo"
  path   = "foo2"
}

stack "foo3" {
  source = "../stacks/foo"
  path   = "foo3"
}
`
	liveStackPath := filepath.Join(liveDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(liveStackPath, []byte(liveStackConfig), 0644))

	fooStackConfig := `locals {
  final_stack = find_in_parent_folders("stacks/final")
}

stack "final" {
  source = local.final_stack
  path   = "final"
}

stack "final2" {
  source = local.final_stack
  path   = "final2"
}

stack "final3" {
  source = local.final_stack
  path   = "final3"
}
`
	fooStackPath := filepath.Join(stacksDir, "foo", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(fooStackPath, []byte(fooStackConfig), 0644))

	finalStackConfig := `locals {
  final_unit = find_in_parent_folders("units/final")
}

unit "final" {
  source = local.final_unit
  path   = "final"
}
`
	finalStackPath := filepath.Join(stacksDir, "final", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(finalStackPath, []byte(finalStackConfig), 0644))

	finalUnitPath := filepath.Join(unitsDir, "final", config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(finalUnitPath, []byte(``), 0644))

	finalMainTfPath := filepath.Join(unitsDir, "final", "main.tf")
	require.NoError(t, os.WriteFile(finalMainTfPath, []byte(``), 0644))
}

// verifyGeneratedUnits checks that some units were generated correctly
func verifyGeneratedUnits(t *testing.T, stackDir string) {
	t.Helper()

	var (
		unitDirs  []string
		stackDirs []string
	)

	err := filepath.WalkDir(stackDir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == "terragrunt.hcl" {
			unitDir := filepath.Dir(path)
			unitDirs = append(unitDirs, unitDir)
		}

		if !info.IsDir() && info.Name() == "terragrunt.stack.hcl" {
			stackDir := filepath.Dir(path)
			stackDirs = append(stackDirs, stackDir)
		}

		return nil
	})
	require.NoError(t, err)

	require.Len(t, unitDirs, 9, "Expected exactly 9 generated units")
	require.Len(t, stackDirs, 12, "Expected exactly 12 generated stacks")
}

// findStackFiles recursively finds all terragrunt.stack.hcl files in a directory
func findStackFiles(t *testing.T, dir string) []string {
	t.Helper()

	var stackFiles []string

	err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "terragrunt.stack.hcl") {
			stackFiles = append(stackFiles, path)
		}

		return nil
	})

	require.NoError(t, err)

	return stackFiles
}
