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
	gcpKMSKeyID                          = "projects/terragrunt-test/locations/global/keyRings/terragrunt-test/cryptoKeys/terragrunt-test-key"
	awsKMSKeyID                          = "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012"
	stateFile                            = "terraform.tfstate"
	awsKMSKeyRegion                      = "us-west-2"
)

func TestTofuStateEncryptionPBKDF2(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionPBKDF2)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionPBKDF2)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", workDir))
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}

func TestTofuStateEncryptionGCPKMS(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionGCPKMS)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionGCPKMS)
	configPath := util.JoinPath(workDir, "terragrunt.hcl")

	helpers.CopyAndFillMapPlaceholders(t, configPath, configPath, map[string]string{
		"__FILL_IN_KMS_KEY_ID__": gcpKMSKeyID,
	})

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", workDir))
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

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", workDir))
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
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
	assert.NoError(t, err, "Error unmarshalling the state file '%s'", fileName)

	encryptedData, exists := result["encrypted_data"]
	assert.True(t, exists, "The key 'encrypted_data' should exist in the state '%s'", fileName)

	// Check if the encrypted_data is base64 encoded (common for AES-256 encrypted data)
	encryptedDataStr, ok := encryptedData.(string)
	assert.True(t, ok, "The value of 'encrypted_data' should be a string")

	_, err = base64.StdEncoding.DecodeString(encryptedDataStr)
	assert.NoError(t, err, "The value of 'encrypted_data' should be base64 encoded, indicating AES-256 encryption")
}
