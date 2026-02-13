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
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

	// Create options
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

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
	client, err := azurehelper.CreateBlobServiceClient(ctx, testLogger, terragruntOptions, backendConfig)
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
	jsonBytes, err := getTerragruntOutputJSONFromRemoteStateAzurerm(ctx, testLogger, terragruntOptions, remoteState)
	require.NoError(t, err)

	// Parse and verify outputs
	var stateFileObj map[string]interface{}
	err = json.Unmarshal(jsonBytes, &stateFileObj)
	require.NoError(t, err)

	stateOutputsMap, outputsOk := stateFileObj["outputs"].(map[string]interface{})
	require.True(t, outputsOk, "outputs section not found in state file")

	// Verify the output value
	testOutputIface, testOutputOk := stateOutputsMap["test_output"]
	require.True(t, testOutputOk, "test_output not found in state file")

	testOutputMap, testOutputMapOk := testOutputIface.(map[string]interface{})
	require.True(t, testOutputMapOk, "test_output is not a map")

	valueIface, valueOk := testOutputMap["value"]
	require.True(t, valueOk, "value not found in test_output")

	valueStr, valueStrOk := valueIface.(string)
	require.True(t, valueStrOk, "value is not a string")
	assert.Equal(t, "azure_test_value", valueStr)
}

// getTerragruntOutputJSONFromRemoteStateAzurerm pulls the output directly from an Azure storage without calling Terraform
// This is the test version of the function from config/dependency.go
func getTerragruntOutputJSONFromRemoteStateAzurerm(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, remoteState *remotestate.RemoteState) ([]byte, error) {
	// Create Azure blob client from the configuration
	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, remoteState.BackendConfig)
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
	container := containerName
	keyPtr := &key
	containerPtr := &container
	input := &azurehelper.GetObjectInput{
		Container: containerPtr,
		Key:       keyPtr,
	}

	output, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, errors.Errorf("error reading terraform state blob %s from container %s: %w", key, containerName, err)
	}

	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, errors.Errorf("error reading response body: %w", err)
	}

	return data, nil
}
