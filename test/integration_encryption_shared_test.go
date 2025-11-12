package test_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	stateFile = "tofu.tfstate" //nolint:unused
)

// Check the statefile contains an encrypted_data key
// and that the encrypted_data is base64 encoded
func validateStateIsEncrypted(t *testing.T, fileName string, path string) { //nolint:unused
	t.Helper()

	filePath := filepath.Join(path, fileName)
	file, err := os.Open(filePath)
	require.NoError(t, err)

	defer file.Close()

	byteValue, err := io.ReadAll(file)
	require.NoError(t, err)

	var result map[string]any

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
