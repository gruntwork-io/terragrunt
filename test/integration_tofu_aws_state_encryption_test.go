//go:build aws && tofu

package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureTofuStateEncryptionPBKDF2 = "fixtures/tofu-state-encryption/pbkdf2"
	testFixtureTofuStateEncryptionGCPKMS = "fixtures/tofu-state-encryption/gcp-kms"
	testFixtureTofuStateEncryptionAWSKMS = "fixtures/tofu-state-encryption/aws-kms"
	testFixtureRenderJSONWithEncryption  = "fixtures/render-json-with-encryption"
	gcpKMSKeyID                          = "projects/terragrunt-test/locations/global/keyRings/terragrunt-test/cryptoKeys/terragrunt-test-key"
	awsKMSKeyID                          = "7a8b0c4e-ff3c-49d0-93ba-15e3ca3488fb"
	awsKMSKeyRegion                      = "us-east-1"
)

func TestTofuStateEncryptionPBKDF2(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionPBKDF2)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionPBKDF2)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+workDir)
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}

func TestTofuStateEncryptionGCPKMS(t *testing.T) {
	t.Skip("Skipping test as the GCP KMS key is not available. You have to setup your own GCP KMS key to run this test.")
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionGCPKMS)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionGCPKMS)
	configPath := util.JoinPath(workDir, "terragrunt.hcl")

	helpers.CopyAndFillMapPlaceholders(t, configPath, configPath, map[string]string{
		"__FILL_IN_KMS_KEY_ID__": gcpKMSKeyID,
	})

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+workDir)
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}

func TestTofuStateEncryptionAWSKMS(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionAWSKMS)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionAWSKMS)
	configPath := util.JoinPath(workDir, "terragrunt.hcl")

	helpers.CopyAndFillMapPlaceholders(t, configPath, configPath, map[string]string{
		"__FILL_IN_KMS_KEY_ID__": awsKMSKeyID,
		"__FILL_IN_AWS_REGION__": awsKMSKeyRegion,
	})

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+workDir)
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}

func TestTofuRenderJSONConfigWithEncryption(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONWithEncryption)
	workDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONWithEncryption)
	mainPath := util.JoinPath(workDir, "main")
	jsonOut := filepath.Join(mainPath, "terragrunt_rendered.json")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+workDir+" -- apply -auto-approve")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --non-interactive --log-level trace --working-dir %s --json-out %s", mainPath, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &rendered))

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
				"encryption": map[string]any{
					"key_provider": "pbkdf2",
					"passphrase":   "correct-horse-battery-staple",
				},
				"disable_init":                    false,
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

// This will eventually be the only test for rendering JSON config with encryption
func TestTofuRenderJSONConfigWithEncryptionExp(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONWithEncryption)
	workDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONWithEncryption)
	mainPath := util.JoinPath(workDir, "main")
	jsonOut := filepath.Join(mainPath, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+workDir+" -- apply -auto-approve")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json  -w --non-interactive --log-level trace --working-dir %s --out %s", mainPath, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &rendered))

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
				"encryption": map[string]any{
					"key_provider": "pbkdf2",
					"passphrase":   "correct-horse-battery-staple",
				},
				"disable_init":                    false,
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
