package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fixtureMultiIncludeDependency = "fixtures/multiinclude-dependency"
	fixtureRenderJSON             = "fixtures/render-json"
	fixtureRenderJSONRegression   = "fixtures/render-json-regression"
)

func TestDebugGeneratedInputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --log-level trace --inputs-debug --working-dir "+rootPath, &stdout, &stderr),
	)

	debugFile := util.JoinPath(rootPath, helpers.TerragruntDebugFile)
	assert.True(t, util.FileExists(debugFile))

	if helpers.IsWindows() {
		// absolute path test on Windows
		assert.Contains(t, stderr.String(), fmt.Sprintf("-chdir=\"%s\"", rootPath))
	} else {
		assert.Contains(t, stderr.String(), fmt.Sprintf("-chdir=\"%s\"", getPathRelativeTo(t, rootPath, rootPath)))
	}

	// If the debug file is generated correctly, we should be able to run terraform apply using the generated var file
	// without going through terragrunt.
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	require.NoError(t, err)

	mockOptions.WorkingDir = rootPath

	l := logger.CreateLogger()

	require.NoError(
		t,
		tf.RunCommand(t.Context(), l, mockOptions, "apply", "-auto-approve", "-var-file", debugFile),
	)

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)

	// Also make sure the undefined variable is not included in the json file
	debugJSONContents, err := os.ReadFile(debugFile)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(debugJSONContents, &data))
	_, isDefined := data["undefined_var"]
	assert.False(t, isDefined)
}

func TestTerragruntInputsWithDashes(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt init --working-dir=%s --log-level=debug", rootPath))
}

func TestTerragruntValidateInputs(t *testing.T) {
	t.Parallel()

	moduleDirs, err := filepath.Glob(filepath.Join("fixtures/validate-inputs", "*"))
	require.NoError(t, err)

	for _, module := range moduleDirs {
		name := filepath.Base(module)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nameDashSplit := strings.Split(name, "-")
			helpers.RunTerragruntValidateInputs(t, module, []string{"--strict"}, nameDashSplit[0] == "success")
		})
	}
}

func TestTerragruntValidateInputsWithCLIVars(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	args := []string{"-var=input=from_env"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithCLIVarFile(t *testing.T) {
	t.Parallel()

	curdir, err := os.Getwd()
	require.NoError(t, err)

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	args := []string{fmt.Sprintf("-var-file=%s/fixtures/validate-inputs/success-var-file/varfiles/main.tfvars", curdir)}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictMode(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"--strict-validate"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictModeDisabledAndUnusedVar(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"-var=testvariable=testvalue"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictModeEnabledAndUnusedVar(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"-var=testvariable=testvalue", "--strict"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, false)
}

func TestTerragruntValidateInputsWithStrictModeEnabledAndUnusedInputs(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-unused-inputs")
	helpers.CleanupTerraformFolder(t, moduleDir)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, moduleDir))
	rootPath := util.JoinPath(tmpEnvPath, moduleDir)

	args := []string{"--strict"}
	helpers.RunTerragruntValidateInputs(t, rootPath, args, false)
}

func TestTerragruntValidateInputsWithStrictModeDisabledAndUnusedInputs(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-unused-inputs")
	helpers.CleanupTerraformFolder(t, moduleDir)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, moduleDir))
	rootPath := util.JoinPath(tmpEnvPath, moduleDir)

	args := []string{}
	helpers.RunTerragruntValidateInputs(t, rootPath, args, true)
}

func TestRenderJSONConfig(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, fixtureRenderJSON)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSON)
	rootPath := util.JoinPath(tmpEnvPath, fixtureRenderJSON)

	fixtureRenderJSONMainModulePath := filepath.Join(rootPath, "main")
	fixtureRenderJSONDepModulePath := filepath.Join(rootPath, "dep")

	helpers.CleanupTerraformFolder(t, fixtureRenderJSONMainModulePath)
	helpers.CleanupTerraformFolder(t, fixtureRenderJSONDepModulePath)

	tmpDir := t.TempDir()
	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- apply -auto-approve")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --non-interactive --log-level trace --working-dir %s --json-out %s", fixtureRenderJSONMainModulePath, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	// clean jsonBytes to remove any trailing newlines
	cleanString := util.CleanString(string(jsonBytes))

	var rendered map[string]any
	require.NoError(t, json.Unmarshal([]byte(cleanString), &rendered))

	// Make sure all terraform block is visible
	terraformBlock, hasTerraform := rendered["terraform"]
	if assert.True(t, hasTerraform) {
		source, hasSource := terraformBlock.(map[string]any)["source"]
		assert.True(t, hasSource)
		assert.Equal(t, "./module", source)
	}

	// Make sure included remoteState is rendered out
	remoteState, hasRemoteState := rendered["remote_state"]
	if assert.True(t, hasRemoteState) {
		assert.Equal(
			t,
			map[string]any{
				"backend": "local",
				"generate": map[string]any{
					"path":      "backend.tf",
					"if_exists": "overwrite_terragrunt",
				},
				"config": map[string]any{
					"path": "foo.tfstate",
				},
				"disable_init":                    false,
				"encryption":                      nil,
				"disable_dependency_optimization": false,
			},
			remoteState.(map[string]any),
		)
	}

	// Make sure dependency blocks are rendered out
	dependencyBlocks, hasDependency := rendered["dependency"]
	if assert.True(t, hasDependency) {
		assert.Equal(
			t,
			map[string]any{
				"dep": map[string]any{
					"name":         "dep",
					"config_path":  "../dep",
					"outputs":      nil,
					"inputs":       nil,
					"mock_outputs": nil,
					"enabled":      nil,
					"mock_outputs_allowed_terraform_commands": nil,
					"mock_outputs_merge_strategy_with_state":  nil,
					"mock_outputs_merge_with_state":           nil,
					"skip":                                    nil,
				},
			},
			dependencyBlocks.(map[string]any),
		)
	}

	// Make sure included generate block is rendered out
	generateBlocks, hasGenerate := rendered["generate"]
	if assert.True(t, hasGenerate) {
		assert.Equal(
			t,
			map[string]any{
				"provider": map[string]any{
					"path":              "provider.tf",
					"comment_prefix":    "# ",
					"disable_signature": false,
					"disable":           false,
					"if_exists":         "overwrite_terragrunt",
					"if_disabled":       "skip",
					"hcl_fmt":           nil,
					"contents": `provider "aws" {
  region = "us-east-1"
}
`,
				},
			},
			generateBlocks.(map[string]any),
		)
	}

	// Make sure all inputs are merged together
	inputsBlock, hasInputs := rendered["inputs"]
	if assert.True(t, hasInputs) {
		assert.Equal(
			t,
			map[string]any{
				"env":        "qa",
				"name":       "dep",
				"type":       "main",
				"aws_region": "us-east-1",
			},
			inputsBlock.(map[string]any),
		)
	}
}

func TestRenderJSONConfigWithIncludesDependenciesAndLocals(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+workDir+" -- apply -auto-approve")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --non-interactive --log-level trace --working-dir %s --json-out ", workDir)+jsonOut)

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &rendered))

	// Make sure all terraform block is visible
	terraformBlock, hasTerraform := rendered["terraform"]
	if assert.True(t, hasTerraform) {
		source, hasSource := terraformBlock.(map[string]any)["source"]
		assert.True(t, hasSource)
		assert.Equal(t, "./foo", source)
	}

	// Make sure top level locals are rendered out
	locals, hasLocals := rendered["locals"]
	if assert.True(t, hasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			locals.(map[string]any),
		)
	}

	// Make sure included dependency block is rendered out, and with the outputs rendered
	dependencyBlocks, hasDependency := rendered["dependency"]
	if assert.True(t, hasDependency) {
		assert.Equal(
			t,
			map[string]any{
				"baz": map[string]any{
					"name":         "baz",
					"config_path":  "./baz",
					"outputs":      nil,
					"inputs":       nil,
					"mock_outputs": nil,
					"enabled":      nil,
					"mock_outputs_allowed_terraform_commands": nil,
					"mock_outputs_merge_strategy_with_state":  nil,
					"mock_outputs_merge_with_state":           nil,
					"skip":                                    nil,
				},
			},
			dependencyBlocks.(map[string]any),
		)
	}

	// Make sure generate block is rendered out
	generateBlocks, hasGenerate := rendered["generate"]
	if assert.True(t, hasGenerate) {
		assert.Equal(
			t,
			map[string]any{
				"provider": map[string]any{
					"path":              "provider.tf",
					"comment_prefix":    "# ",
					"disable_signature": false,
					"disable":           false,
					"if_exists":         "overwrite",
					"if_disabled":       "skip",
					"hcl_fmt":           nil,
					"contents":          "# This is just a test",
				},
			},
			generateBlocks.(map[string]any),
		)
	}

	// Make sure all inputs are merged together
	inputsBlock, hasInputs := rendered["inputs"]
	if assert.True(t, hasInputs) {
		assert.Equal(
			t,
			map[string]any{
				"foo":       "bar",
				"baz":       "blah",
				"another":   "baz",
				"from_root": "Hi",
			},
			inputsBlock.(map[string]any),
		)
	}
}

func TestRenderJSONConfigRunAll(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	// NOTE: bar is not rendered out because it is considered a parent terragrunt.hcl config.

	bazJSONOut := filepath.Join(workDir, "baz", "terragrunt.rendered.json")
	rootChildJSONOut := filepath.Join(workDir, "terragrunt.rendered.json")

	defer os.Remove(bazJSONOut)
	defer os.Remove(rootChildJSONOut)

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+workDir+" -- apply -auto-approve")

	helpers.RunTerragrunt(t, "terragrunt render --all --json -w --non-interactive --log-level trace --working-dir "+workDir)

	bazJSONBytes, err := os.ReadFile(bazJSONOut)
	require.NoError(t, err)

	var bazRendered map[string]any
	require.NoError(t, json.Unmarshal(bazJSONBytes, &bazRendered))

	// Make sure top level locals are rendered out
	bazLocals, bazHasLocals := bazRendered["locals"]
	if assert.True(t, bazHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"self": "baz",
			},
			bazLocals.(map[string]any),
		)
	}

	rootChildJSONBytes, err := os.ReadFile(rootChildJSONOut)
	require.NoError(t, err)

	var rootChildRendered map[string]any
	require.NoError(t, json.Unmarshal(rootChildJSONBytes, &rootChildRendered))

	// Make sure top level locals are rendered out
	rootChildLocals, rootChildHasLocals := rootChildRendered["locals"]
	if assert.True(t, rootChildHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			rootChildLocals.(map[string]any),
		)
	}
}

func TestRenderJSONConfigRunAllWithCLIRedesign(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	// NOTE: bar is not rendered out because it is considered a parent terragrunt.hcl config.

	bazJSONOut := filepath.Join(workDir, "baz", "terragrunt.rendered.json")
	rootChildJSONOut := filepath.Join(workDir, "terragrunt.rendered.json")

	defer os.Remove(bazJSONOut)
	defer os.Remove(rootChildJSONOut)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --log-level trace --working-dir "+workDir)

	helpers.RunTerragrunt(t, "terragrunt render --all --json -w --non-interactive --log-level trace --working-dir "+workDir)

	bazJSONBytes, err := os.ReadFile(bazJSONOut)
	require.NoError(t, err)

	var bazRendered map[string]any
	require.NoError(t, json.Unmarshal(bazJSONBytes, &bazRendered))

	// Make sure top level locals are rendered out
	bazLocals, bazHasLocals := bazRendered["locals"]
	if assert.True(t, bazHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"self": "baz",
			},
			bazLocals.(map[string]any),
		)
	}

	rootChildJSONBytes, err := os.ReadFile(rootChildJSONOut)
	require.NoError(t, err)

	var rootChildRendered map[string]any
	require.NoError(t, json.Unmarshal(rootChildJSONBytes, &rootChildRendered))

	// Make sure top level locals are rendered out
	rootChildLocals, rootChildHasLocals := rootChildRendered["locals"]
	if assert.True(t, rootChildHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			rootChildLocals.(map[string]any),
		)
	}
}

func TestDependencyGraphWithMultiInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, fixtureMultiIncludeDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureMultiIncludeDependency)
	rootPath := util.JoinPath(tmpEnvPath, fixtureMultiIncludeDependency)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt dag graph --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)
	stdoutStr := stdout.String()

	assert.Contains(t, stdoutStr, `"main" -> "depa";`)
	assert.Contains(t, stdoutStr, `"main" -> "depb";`)
	assert.Contains(t, stdoutStr, `"main" -> "depc";`)
}
