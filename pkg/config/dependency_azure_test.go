//go:build azure

package config_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectStateAccessAzurerm(t *testing.T) {
	t.Parallel()

	// Skip test if we're not running on Azure environment
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" {
		t.Skip("Skipping Azure test as TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT is not set")
	}

	// Create a mock state file with known outputs
	stateOutputs := `{
        "version": 4,
        "terraform_version": "1.3.7",
        "serial": 1,
        "lineage": "12345678-1234-1234-1234-123456789012",
        "outputs": {
            "test_output": {
                "value": "azure_test_value",
                "type": "string"
            }
        },
        "resources": []
    }`

	// Get Azure test config
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	containerName := "terragrunt-test-container-" + strings.ToLower(uniqueID)
	blobName := "terraform.tfstate"

	// Create a logger for testing
	testLogger := logger.CreateLogger()

	// Setup remote state config
	backendConfig := map[string]interface{}{
		"storage_account_name": storageAccount,
		"container_name":       containerName,
		"key":                  blobName,
		"use_azuread_auth":     true,
	}

	remoteStateConfig := &remotestate.Config{
		BackendName:   "azurerm",
		BackendConfig: backendConfig,
	}

	remoteState := remotestate.New(remoteStateConfig)

	// Create Azure client - use t.Context() instead of context.Background()
	ctx := t.Context()
	client, err := azurehelper.CreateBlobServiceClient(ctx, testLogger, backendConfig)
	require.NoError(t, err)

	// Setup - Create container and upload state file
	err = client.CreateContainerIfNecessary(ctx, testLogger, containerName)
	require.NoError(t, err)

	defer func() {
		err = client.DeleteContainer(ctx, testLogger, containerName)
		require.NoError(t, err)
	}()

	err = client.UploadBlob(ctx, testLogger, containerName, blobName, []byte(stateOutputs))
	require.NoError(t, err)

	// Test direct state access
	jsonBytes, err := getTerragruntOutputJSONFromRemoteStateAzurerm(ctx, testLogger, remoteState)
	require.NoError(t, err)

	// Parse and verify outputs — the function returns only the "outputs" section
	var outputsMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &outputsMap)
	require.NoError(t, err)

	// Verify the output value
	testOutputIface, testOutputOk := outputsMap["test_output"]
	require.True(t, testOutputOk, "test_output not found in outputs")

	testOutputMap, testOutputMapOk := testOutputIface.(map[string]interface{})
	require.True(t, testOutputMapOk, "test_output is not a map")

	valueIface, valueOk := testOutputMap["value"]
	require.True(t, valueOk, "value not found in test_output")

	valueStr, valueStrOk := valueIface.(string)
	require.True(t, valueStrOk, "value is not a string")
	assert.Equal(t, "azure_test_value", valueStr)
}

// getTerragruntOutputJSONFromRemoteStateAzurerm is a mock/stub that simulates the production
// function from config/dependency.go for integration-style testing. It fetches state from Azure
// Storage, extracts the "outputs" section, and re-marshals it — matching production semantics.
func getTerragruntOutputJSONFromRemoteStateAzurerm(ctx context.Context, l log.Logger, remoteState *remotestate.RemoteState) ([]byte, error) {
	// Create Azure blob client from the configuration
	client, err := azurehelper.CreateBlobServiceClient(ctx, l, remoteState.BackendConfig)
	if err != nil {
		return nil, err
	}

	// Extract required configuration values
	containerName := remoteState.BackendConfig["container_name"].(string)
	key := remoteState.BackendConfig["key"].(string)

	// Check if container exists
	exists, err := client.ContainerExists(ctx, containerName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.Errorf("Azure container %s does not exist", containerName)
	}

	// Get the state file blob content
	input := &azurehelper.GetObjectInput{
		Container: &containerName,
		Key:       &key,
	}

	output, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, errors.Errorf("error reading terraform state blob %s from container %s: %w", key, containerName, err)
	}

	defer output.Body.Close()
	stateBytes, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, errors.Errorf("error reading response body: %w", err)
	}

	// Parse state and extract outputs — matching production code semantics
	var state struct {
		Outputs map[string]interface{} `json:"outputs"`
	}
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		return nil, errors.Errorf("error parsing state file JSON: %w", err)
	}

	return json.Marshal(state.Outputs)
}
