//go:build tf

package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFDebugGeneratedInputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --log-level debug --inputs-debug --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// Debug file is created in the original config directory
	debugFile := filepath.Join(rootPath, helpers.TerragruntDebugFile)
	assert.True(t, util.FileExists(debugFile))

	// Find cache directory for running terraform
	cacheWorkingDir := helpers.FindCacheWorkingDir(t, rootPath)
	require.NotEmpty(t, cacheWorkingDir, "Should find cache working directory")

	// If the debug file is generated correctly, we should be able to run terraform apply using the generated var file
	// without going through terragrunt.
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	require.NoError(t, err)

	mockOptions.WorkingDir = cacheWorkingDir

	l := logger.CreateLogger()

	require.NoError(
		t,
		tf.RunCommand(
			t.Context(),
			l,
			venv.OSVenv(),
			configbridge.TFRunOptsFromOpts(mockOptions),
			"apply", "-auto-approve", "-var-file", debugFile,
		),
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	validateInputs(t, outputs)

	// Also make sure the undefined variable is not included in the json file
	debugJSONContents, err := os.ReadFile(debugFile)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(debugJSONContents, &data))
	_, isDefined := data["undefined_var"]
	assert.False(t, isDefined)
}

func TestTFTerragruntInputsWithDashes(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf("terragrunt init --working-dir=%s --log-level=debug", rootPath),
	)
}

func TestTFRenderJSONConfig(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, fixtureRenderJSON)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSON)
	rootPath := filepath.Join(tmpEnvPath, fixtureRenderJSON)

	fixtureRenderJSONMainModulePath := filepath.Join(rootPath, "main")
	fixtureRenderJSONDepModulePath := filepath.Join(rootPath, "dep")

	helpers.CleanupTerraformFolder(t, fixtureRenderJSONMainModulePath)
	helpers.CleanupTerraformFolder(t, fixtureRenderJSONDepModulePath)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt render --json -w --non-interactive --working-dir %s --json-out %s",
			fixtureRenderJSONMainModulePath,
			jsonOut,
		),
	)

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	// clean jsonBytes to remove any trailing newlines
	cleanString := strings.TrimSpace(string(jsonBytes))

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
