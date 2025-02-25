//go:build tofu

package test_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	awsKMSKeyID                          = "bd372994-d969-464a-a261-6cc850c58a92"
	stateFile                            = "terraform.tfstate"
	awsKMSKeyRegion                      = "us-east-1"
)

var testFixtureRenderJSONWithEncryptionMainModulePath = filepath.Join(testFixtureRenderJSONWithEncryption, "main")

func TestTofuStateEncryptionPBKDF2(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionPBKDF2)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionPBKDF2)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+workDir)
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

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+workDir)
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

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+workDir)
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}

func TestTofuRenderJSONConfigWithEncryption(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "terragrunt-render-json-*")
	require.NoError(t, err)
	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")
	defer os.RemoveAll(tmpDir)

	helpers.CleanupTerraformFolder(t, fixtureRenderJSONMainModulePath)
	helpers.CleanupTerraformFolder(t, fixtureRenderJSONDepModulePath)

	helpers.RunTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+testFixtureRenderJSONWithEncryption)
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render-json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir %s --terragrunt-json-out %s", testFixtureRenderJSONWithEncryptionMainModulePath, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var rendered map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &rendered))

	// Make sure all terraform block is visible
	terraformBlock, hasTerraform := rendered["terraform"]
	if assert.True(t, hasTerraform) {
		source, hasSource := terraformBlock.(map[string]interface{})["source"]
		assert.True(t, hasSource)
		assert.Equal(t, "./module", source)
	}

	// Make sure included remoteState is rendered out
	remoteState, hasRemoteState := rendered["remote_state"]
	if assert.True(t, hasRemoteState) {
		assert.Equal(
			t,
			map[string]interface{}{
				"backend": "local",
				"generate": map[string]interface{}{
					"path":      "backend.tf",
					"if_exists": "overwrite_terragrunt",
				},
				"config": map[string]interface{}{
					"path": "foo.tfstate",
				},
				"encryption": map[string]interface{}{
					"key_provider": "pbkdf2",
					"passphrase":   "correct-horse-battery-staple",
				},
				"disable_init":                    false,
				"disable_dependency_optimization": false,
			},
			remoteState.(map[string]interface{}),
		)
	}

	// Make sure dependency blocks are rendered out
	dependencyBlocks, hasDependency := rendered["dependency"]
	if assert.True(t, hasDependency) {
		assert.Equal(
			t,
			map[string]interface{}{
				"dep": map[string]interface{}{
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
			dependencyBlocks.(map[string]interface{}),
		)
	}

	// Make sure included generate block is rendered out
	generateBlocks, hasGenerate := rendered["generate"]
	if assert.True(t, hasGenerate) {
		assert.Equal(
			t,
			map[string]interface{}{
				"provider": map[string]interface{}{
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
			generateBlocks.(map[string]interface{}),
		)
	}

	// Make sure all inputs are merged together
	inputsBlock, hasInputs := rendered["inputs"]
	if assert.True(t, hasInputs) {
		assert.Equal(
			t,
			map[string]interface{}{
				"env":        "qa",
				"name":       "dep",
				"type":       "main",
				"aws_region": "us-east-1",
			},
			inputsBlock.(map[string]interface{}),
		)
	}
}

// Check the statefile contains an encrypted_data key
// and that the encrypted_data is base64 encoded
func validateStateIsEncrypted(t *testing.T, fileName string, path string) {
	t.Helper()

	filePath := filepath.Join(path, fileName)
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(byteValue, &result)
	require.NoError(t, err, "Error unmarshalling the state file '%s'", fileName)

	encryptedData, exists := result["encrypted_data"]
	assert.True(t, exists, "The key 'encrypted_data' should exist in the state '%s'", fileName)

	// Check if the encrypted_data is base64 encoded (common for AES-256 encrypted data)
	encryptedDataStr, ok := encryptedData.(string)
	assert.True(t, ok, "The value of 'encrypted_data' should be a string")

	_, err = base64.StdEncoding.DecodeString(encryptedDataStr)
	assert.NoError(t, err, "The value of 'encrypted_data' should be base64 encoded, indicating AES-256 encryption")
}
