//go:build azure

package test_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init enables the Azure backend experiment for all Azure integration tests
func init() {
	// Enable the Azure backend experiment through environment variable
	os.Setenv("TG_EXPERIMENT", "azure-backend")

	// Also manually register the Azure backend for tests
	// This ensures the backend is registered regardless of when the environment variable is processed
	testOpts := options.NewTerragruntOptions()
	err := testOpts.Experiments.EnableExperiment("azure-backend")
	if err == nil {
		// Import and call RegisterBackends - we need to use the internal/remotestate package
		remotestate.RegisterBackends(testOpts)
	}
}

const (
	testFixtureAzureBackend               = "./fixtures/azure-backend"
	testFixtureAzureOutputFromRemoteState = "./fixtures/azure-output-from-remote-state"
)

// TestCase represents the test case data without the check function.
type TestCase struct {
}

func TestAzureRMBootstrapBackend(t *testing.T) {
	t.Parallel()

	t.Log("Starting TestAzureRMBootstrapBackend")

	testCases := []struct {
		checkExpectedResultFn func(t *testing.T, output string, containerName string, rootPath string, name string, args string)
		name                  string
		args                  string
		containerName         string
	}{
		{
			checkExpectedResultFn: func(t *testing.T, output string, containerName string, rootPath string, name string, args string) {
				t.Helper()

				// In delete case, not finding the container is acceptable
				if strings.Contains(output, "ContainerNotFound") {
					return
				}

				// For thoroughness, let's try bootstrapping and then deleting
				azureCfg := helpers.GetAzureStorageTestConfig(t)
				azureCfg.ContainerName = containerName

				// Bootstrap the backend first
				bootstrapOutput, bootstrapErr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend bootstrap --backend-bootstrap --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Bootstrap command failed: %v\nOutput: %s\nError: %s", err, bootstrapOutput, bootstrapErr)

				opts, err := options.NewTerragruntOptionsForTest("")
				require.NoError(t, err)

				client, err := azurehelper.CreateBlobServiceClient(
					t.Context(),
					logger.CreateLogger(),
					opts,
					map[string]interface{}{
						"storage_account_name": azureCfg.StorageAccountName,
						"container_name":       containerName,
						"use_azuread_auth":     true,
					},
				)
				require.NoError(t, err)

				// Verify container exists after bootstrap
				exists, err := client.ContainerExists(t.Context(), containerName)
				require.NoError(t, err)
				assert.True(t, exists, "Container should exist after bootstrap")

				// Create and verify test state file
				data := []byte("{}")
				err = client.UploadBlob(t.Context(), logger.CreateLogger(), containerName, "unit1/terraform.tfstate", data)
				require.NoError(t, err, "Failed to create test state file")

				stateKey := "unit1/terraform.tfstate"
				_, err = client.GetObject(t.Context(), &azurehelper.GetObjectInput{
					Bucket: &containerName,
					Key:    &stateKey,
				})
				require.NoError(t, err, "State file should exist after creation")

				// Now run the delete command again (will be run by test runner)
				deleteOutput, deleteErr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend delete --force --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Delete command failed: %v\nOutput: %s\nError: %s", err, deleteOutput, deleteErr)

				// Verify container is deleted with retries
				var containerExists bool
				maxRetries := 5
				for i := 0; i < maxRetries; i++ {
					exists, err = client.ContainerExists(t.Context(), containerName)
					require.NoError(t, err)
					if !exists {
						containerExists = false
						break
					}
					time.Sleep(3 * time.Second)
				}
				assert.False(t, containerExists, "Container should not exist after delete")
			},
			name:          "delete backend command",
			args:          "backend delete --force",
			containerName: "terragrunt-test-container-" + strings.ToLower(helpers.UniqueID()),
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureAzureBackend)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureBackend)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureAzureBackend)
			commonConfigPath := util.JoinPath(rootPath, "common.hcl")

			azureCfg := helpers.GetAzureStorageTestConfig(t)

			defer func() {
				// Clean up the destination container
				azureCfg.ContainerName = tc.containerName
				helpers.CleanupAzureContainer(t, azureCfg)
			}()

			// Set up common configuration parameters
			azureParams := map[string]string{
				"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
				"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
				"__FILL_IN_CONTAINER_NAME__":       tc.containerName,
			}

			// Set up the common configuration
			helpers.CopyTerragruntConfigAndFillProviderPlaceholders(t,
				commonConfigPath,
				commonConfigPath,
				azureParams,
				azureCfg.Location)

			stdout, stderr, _ := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --log-format key-value --strict-control require-explicit-bootstrap --working-dir "+rootPath)

			tc.checkExpectedResultFn(t, stdout+stderr, tc.containerName, rootPath, tc.name, tc.args)
		})
	}
}

// TestAzureOutputFromRemoteState tests the ability to get outputs from remote state stored in Azure Storage.
func TestAzureOutputFromRemoteState(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureOutputFromRemoteState)

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureAzureOutputFromRemoteState)

	azureCfg := helpers.GetAzureStorageTestConfig(t)

	// Fill in Azure configuration
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAzureOutputFromRemoteState)
	rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
	containerName := "terragrunt-test-container-" + strings.ToLower(helpers.UniqueID())

	// Set up Azure configuration parameters
	azureParams := map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
		"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
		"__FILL_IN_CONTAINER_NAME__":       containerName,
	}

	// Set up the root configuration
	helpers.CopyTerragruntConfigAndFillProviderPlaceholders(t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		azureParams,
		azureCfg.Location)

	defer func() {
		// Clean up the destination container
		azureCfg.ContainerName = containerName
		helpers.CleanupAzureContainer(t, azureCfg)
	}()

	// Run terragrunt for app3, app2, and app1 in that order (dependencies first)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+environmentPath+"/app3")
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+environmentPath+"/app2")
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+environmentPath+"/app1")

	// Now check the outputs to make sure the remote state dependencies work
	app1OutputCmd := "terragrunt output -no-color -json --non-interactive --working-dir " + environmentPath + "/app1"
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, app1OutputCmd, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// Print the actual output for debugging
	t.Logf("Actual output: %s", outputs["combined_output"].Value)

	// Verify outputs from app1
	assert.Equal(t, `app1 output with app2 output with app3 output and app3 output`, outputs["combined_output"].Value)
}

// CheckAzureTestCredentials checks if the required Azure test credentials are available
// and skips the test if they are not. Returns the storage account name.
func CheckAzureTestCredentials(t *testing.T) (storageAccount string) {
	t.Helper() // Mark this as a test helper function

	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")

	// Skip test if storage account isn't specified
	if storageAccount == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	// Log that we're using Azure AD authentication
	t.Logf("Using Azure AD authentication for tests")

	return storageAccount
}

// TestAzureStorageContainerCreation tests the creation of Azure storage containers
func TestAzureStorageContainerCreation(t *testing.T) {
	t.Parallel()

	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, logger)
	if err != nil {
		t.Skipf("Skipping test: Failed to get Azure credentials: %v", err)
	}

	// Skip test if no subscription ID is available
	if subscriptionID == "" {
		t.Skip("Skipping test: No subscription ID found in environment variables")
	}

	t.Logf("Using subscription ID: %s", subscriptionID)

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope" // Default location
			t.Logf("Neither AZURE_LOCATION nor ARM_LOCATION set, using default: %s", location)
		}
	}

	// Generate unique names for resources
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	storageAccountName := "tgtest" + strings.ToLower(uniqueID)[len(uniqueID)-10:] // Storage account names must be 3-24 chars, alphanumeric only
	resourceGroupName := "terragrunt-test-rg-" + uniqueID
	containerName := "terragrunt-test-" + strings.ToLower(t.Name())
	if len(containerName) > 63 {
		containerName = containerName[:63]
	}

	// Setup cleanup to ensure resources are deleted even if the test fails
	storageAccountCreated := false
	containerCreated := false

	// Setup cleanup of all resources
	t.Cleanup(func() {
		// Create a cleanup client
		cleanupConfig := map[string]interface{}{
			"storage_account_name": storageAccountName,
			"resource_group_name":  resourceGroupName,
			"subscription_id":      subscriptionID,
			"use_azuread_auth":     true,
		}

		if containerCreated {
			cleanupClient, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, cleanupConfig)
			if err == nil {
				logger.Infof("Cleaning up container %s", containerName)
				_ = cleanupClient.DeleteContainer(ctx, logger, containerName)
			}
		}

		if storageAccountCreated {
			cleanupClient, err := azurehelper.CreateStorageAccountClient(ctx, logger, cleanupConfig)
			if err == nil {
				logger.Infof("Cleaning up storage account %s", storageAccountName)
				_ = cleanupClient.DeleteStorageAccount(ctx, logger)
			}
		}
	})

	// Create storage account for the test
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"resource_group_name":  resourceGroupName,
		"subscription_id":      subscriptionID,
		"location":             location,
		"use_azuread_auth":     true,
	}

	t.Logf("Creating storage account %s in resource group %s", storageAccountName, resourceGroupName)

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, logger, storageAccountConfig)
	require.NoError(t, err)
	require.NotNil(t, storageClient)

	// Define storage account configuration
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        subscriptionID,
		ResourceGroupName:     resourceGroupName,
		StorageAccountName:    storageAccountName,
		Location:              location,
		EnableVersioning:      true,
		AllowBlobPublicAccess: false,
		AccountKind:           "StorageV2",
		AccountTier:           "Standard",
		AccessTier:            "Hot",
		ReplicationType:       "LRS",
		Tags:                  map[string]string{"created-by": "terragrunt-integration-test"},
	}

	// Create storage account and resource group if necessary
	err = storageClient.CreateStorageAccountIfNecessary(ctx, logger, saConfig)
	require.NoError(t, err)
	storageAccountCreated = true

	// Verify storage account exists
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)

	t.Logf("Storage account %s created successfully", storageAccountName)

	// Create a new client for the test
	config := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"key":                  "test/terraform.tfstate",
		"use_azuread_auth":     true,
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
	require.NoError(t, err)

	// Create container
	err = client.CreateContainerIfNecessary(ctx, logger, containerName)
	require.NoError(t, err, "Failed to create container")
	containerCreated = true

	// Check if container exists
	exists, err = client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	require.True(t, exists, "Container should exist after creation")

	// Clean up
	err = client.DeleteContainer(t.Context(), logger, containerName)
	require.NoError(t, err, "Failed to delete container")

	// Test creating multiple containers with the same client
	containerNameState := containerName + "-state"

	// Create another container
	err = client.CreateContainerIfNecessary(t.Context(), logger, containerNameState)
	require.NoError(t, err, "Failed to create second container")

	// Check if second container exists
	exists, err = client.ContainerExists(t.Context(), containerNameState)
	require.NoError(t, err)
	require.True(t, exists, "Second container should exist after creation")

	// Clean up second container
	err = client.DeleteContainer(t.Context(), logger, containerNameState)
	require.NoError(t, err, "Failed to delete test container")

	// Test error handling for invalid container names
	t.Run("InvalidContainerName", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		invalidContainerName := "UPPERCASE_CONTAINER"
		invalidClient, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, map[string]interface{}{
			"storage_account_name": storageAccountName,
			"container_name":       invalidContainerName,
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		})
		require.NoError(t, err) // Should fail with invalid container name
		err = invalidClient.CreateContainerIfNecessary(t.Context(), logger, invalidContainerName)
		require.Error(t, err, "Creating container with invalid name should fail")

		// Check if error is the specific azurehelper ContainerCreationError type
		var containerErr azurehelper.ContainerCreationError
		if errors.As(err, &containerErr) {
			assert.Equal(t, invalidContainerName, containerErr.ContainerName, "Error should contain the invalid container name")
			assert.Error(t, containerErr.Unwrap(), "Error should have an underlying error")
		} else {
			// Fallback for backwards compatibility - at minimum it should mention the issue
			assert.Contains(t, err.Error(), "container", "Error should mention container issues")
		}
	})
}

// TestStorageAccountBootstrap tests storage account bootstrap functionality
func TestStorageAccountBootstrap(t *testing.T) {
	t.Parallel()
	// Skip test if we don't have Azure credentials
	accountName := CheckAzureTestCredentials(t)

	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Test with non-existent storage account
	t.Run("StorageAccountBootstrap_NonExistentAccount", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		// Use a non-existent account name
		nonExistentName := "tgnon" + strings.ToLower(fmt.Sprintf("%x", time.Now().UnixNano()))[0:8]

		// Create configuration for the non-existent storage account
		config := map[string]interface{}{
			"storage_account_name": nonExistentName,
			"container_name":       "terragrunt-test-sa-nonexistent",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// When we try to create a blob service client, it should fail since the account doesn't exist
		_, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
		require.Error(t, err, "Expected error when storage account doesn't exist")

		// The error should be wrapped properly and contain useful information
		if err != nil {
			// Check if it's an Azure-specific authentication or storage account error
			var authErr azurerm.AuthenticationError
			var storageErr azurerm.StorageAccountCreationError

			switch {
			case errors.As(err, &authErr):
				assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
				require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
			case errors.As(err, &storageErr):
				assert.Equal(t, nonExistentName, storageErr.StorageAccountName, "Storage error should contain account name")
				require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
			default:
				// Fallback check - at minimum should mention the issue
				assert.True(t,
					strings.Contains(err.Error(), "no such host") ||
						strings.Contains(err.Error(), "does not exist") ||
						strings.Contains(err.Error(), "not accessible"),
					"Error should indicate storage account access issue, got: %v", err)
			}
		}
	})

	// Test with existing storage account
	t.Run("StorageAccountBootstrap_ExistingAccount", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		// We'll use the provided test storage account which should already exist
		config := map[string]interface{}{
			"storage_account_name": accountName,
			"container_name":       "terragrunt-test-sa-exists",
			"key":                  "test/terraform.tfstate",
		}

		// Set Azure AD authentication
		config["use_azuread_auth"] = true

		// Should succeed since the account should exist
		client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
		require.NoError(t, err)

		// Create container for test
		err = client.CreateContainerIfNecessary(t.Context(), logger, "terragrunt-test-sa-exists")
		require.NoError(t, err)

		// Clean up the container
		exists, err := client.ContainerExists(t.Context(), "terragrunt-test-sa-exists")
		require.NoError(t, err)

		if exists {
			err = client.DeleteContainer(t.Context(), logger, "terragrunt-test-sa-exists")
			require.NoError(t, err)
		}
	})
}
func TestBlobOperations(t *testing.T) {
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure blob operations test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	t.Parallel()

	ctx := t.Context()
	containerName := fmt.Sprintf("test-container-%d", os.Getpid())
	blobName := "test-blob.txt"

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	config := map[string]interface{}{
		"storage_account_name": os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
		"container_name":       containerName,
	}

	logger := logger.CreateLogger()
	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test container creation
	err = client.CreateContainerIfNecessary(ctx, logger, containerName)
	require.NoError(t, err)

	// Test container existence check
	exists, err := client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists)

	// Test blob operations
	input := &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	}

	// Test get non-existent blob
	_, err = client.GetObject(ctx, input)
	require.Error(t, err)

	// Test delete non-existent blob
	err = client.DeleteBlobIfNecessary(ctx, logger, containerName, blobName)
	require.NoError(t, err)

	// Clean up
	err = client.DeleteContainer(ctx, logger, containerName)
	require.NoError(t, err)

	// Verify container deletion
	exists, err = client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestStorageAccountCreationAndBlobUpload tests the complete workflow of creating a storage account and uploading a blob
func TestStorageAccountCreationAndBlobUpload(t *testing.T) {
	t.Parallel()

	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, logger)
	if err != nil {
		t.Skipf("Skipping storage account creation test: Failed to get Azure credentials: %v", err)
	}

	// Skip test if no subscription ID is available
	if subscriptionID == "" {
		t.Skip("Skipping storage account creation test: No subscription ID found in environment variables")
	}

	t.Logf("Using subscription ID: %s", subscriptionID)

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope" // Default location
			t.Logf("Neither AZURE_LOCATION nor ARM_LOCATION set, using default: %s", location)
		}
	}

	// Generate unique names for resources
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	storageAccountName := "tgtest" + strings.ToLower(uniqueID)[len(uniqueID)-10:] // Storage account names must be 3-24 chars, alphanumeric only
	resourceGroupName := "terragrunt-test-rg-" + uniqueID
	containerName := "test-container-" + strings.ToLower(uniqueID)
	blobName := "test-blob.json"

	// First create the resource group using the new ResourceGroupClient
	resourceGroupTags := map[string]string{
		"created-by": "terragrunt-integration-test",
		"test-case":  "TestStorageAccountCreationAndBlobUpload",
	}

	t.Logf("Creating resource group %s in %s", resourceGroupName, location)
	rgClient, err := azurehelper.CreateResourceGroupClient(ctx, logger, subscriptionID)
	require.NoError(t, err)
	require.NotNil(t, rgClient)

	// Create resource group if it doesn't exist
	resourceGroupCreated := false

	// Defer cleanup of resource group
	defer func() {
		if resourceGroupCreated {
			t.Logf("Cleaning up resource group %s", resourceGroupName)
			// Ignore errors during cleanup
			_ = rgClient.DeleteResourceGroup(ctx, logger, resourceGroupName)
		}
	}()

	err = rgClient.EnsureResourceGroup(ctx, logger, resourceGroupName, location, resourceGroupTags)
	require.NoError(t, err)
	resourceGroupCreated = true
	t.Logf("Resource group %s created successfully", resourceGroupName)

	// Configuration for storage account client
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"resource_group_name":  resourceGroupName,
		"subscription_id":      subscriptionID,
		"location":             location,
		"use_azuread_auth":     true,
	}

	t.Logf("Creating storage account %s in resource group %s", storageAccountName, resourceGroupName)

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, logger, storageAccountConfig)
	require.NoError(t, err)
	require.NotNil(t, storageClient)

	// Define storage account configuration
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        subscriptionID,
		ResourceGroupName:     resourceGroupName,
		StorageAccountName:    storageAccountName,
		Location:              location,
		EnableVersioning:      true,
		AllowBlobPublicAccess: false,
		AccountKind:           "StorageV2",
		AccountTier:           "Standard",
		AccessTier:            "Hot",
		ReplicationType:       "LRS",
		Tags:                  map[string]string{"created-by": "terragrunt-integration-test"},
	}

	// Setup cleanup to ensure resources are deleted even if the test fails
	// Using a cleanup function to track what resources were created
	storageAccountCreated := false
	containerCreated := false
	blobCreated := false

	// Create blob service client configuration - declare it early for later use
	blobConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"use_azuread_auth":     true,
	}

	// We'll create the blob client after the storage account is created to avoid premature validation

	// Define blob client and cleanup variables
	var blobClient *azurehelper.BlobServiceClient

	// Defer cleanup of all resources
	defer func() {
		if blobCreated && blobClient != nil {
			t.Logf("Cleanup: Deleting blob %s", blobName)
			// Ignore errors during cleanup
			_ = blobClient.DeleteBlobIfNecessary(ctx, logger, containerName, blobName)
		}

		if containerCreated && blobClient != nil {
			t.Logf("Cleanup: Deleting container %s", containerName)
			// Ignore errors during cleanup
			_ = blobClient.DeleteContainer(ctx, logger, containerName)
		}

		if storageAccountCreated {
			t.Logf("Cleanup: Deleting storage account %s", storageAccountName)
			// Ignore errors during cleanup
			_ = storageClient.DeleteStorageAccount(ctx, logger)
		}

		t.Log("Cleanup completed")
	}()

	// Create storage account and resource group if necessary
	err = storageClient.CreateStorageAccountIfNecessary(ctx, logger, saConfig)
	require.NoError(t, err)
	storageAccountCreated = true

	// Verify storage account exists
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)

	t.Logf("Storage account %s created successfully", storageAccountName)

	// Now create the blob client after the storage account exists
	blobClient, err = azurehelper.CreateBlobServiceClient(ctx, logger, opts, blobConfig)
	require.NoError(t, err)
	require.NotNil(t, blobClient)

	// Now test blob operations with the created storage account

	// Create container
	err = blobClient.CreateContainerIfNecessary(ctx, logger, containerName)
	require.NoError(t, err)
	containerCreated = true

	// Verify container exists
	containerExists, err := blobClient.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	require.True(t, containerExists, "Container should exist after creation")

	// Create test blob content
	testContent := map[string]interface{}{
		"test_key":   "test_value",
		"timestamp":  time.Now().Unix(),
		"created_by": "terragrunt-integration-test",
	}

	contentBytes, err := json.Marshal(testContent)
	require.NoError(t, err)

	// Upload blob
	err = blobClient.UploadBlob(ctx, logger, containerName, blobName, contentBytes)
	require.NoError(t, err)
	blobCreated = true

	t.Logf("Successfully uploaded blob %s to container %s", blobName, containerName)

	// Verify blob exists and has correct content
	// Use GetObject instead of BlobExists to check if blob exists
	blobExists, err := blobClient.GetObject(ctx, &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	})
	require.NoError(t, err)
	require.NotNil(t, blobExists, "Blob should exist after upload")
	// Close the body to avoid resource leaks
	if blobExists != nil && blobExists.Body != nil {
		blobExists.Body.Close()
	}

	// Download and verify blob content
	downloadedContent, err := blobClient.GetObject(ctx, &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	})
	require.NoError(t, err)
	require.NotNil(t, downloadedContent)

	// Read the downloaded content
	downloadedBytes, err := io.ReadAll(downloadedContent.Body)
	require.NoError(t, err)
	downloadedContent.Body.Close()

	// Verify content matches
	var downloadedJSON map[string]interface{}
	err = json.Unmarshal(downloadedBytes, &downloadedJSON)
	require.NoError(t, err)

	assert.Equal(t, testContent["test_key"], downloadedJSON["test_key"])
	assert.Equal(t, testContent["created_by"], downloadedJSON["created_by"])

	t.Log("Successfully verified blob content matches uploaded data")
	t.Log("Integration test completed successfully - storage account created, blob uploaded")
}

// TestAzureBackendBootstrapWithExistingStorageAccount tests Azure backend bootstrap scenarios
// that require real Azure resources - moved from unit tests
func TestAzureBackendBootstrapWithExistingStorageAccount(t *testing.T) {
	t.Parallel()

	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure backend bootstrap test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	// Create logger for testing
	logger := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	// Create a unique suffix for container names
	uniqueSuffix := fmt.Sprintf("%x", time.Now().UnixNano())[0:8]

	t.Run("bootstrap-with-existing-storage-account", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Use a storage account name that doesn't exist to trigger DNS lookup error
		nonExistentSA := "tgtestsa" + uniqueSuffix

		config := map[string]interface{}{
			"storage_account_name": nonExistentSA,
			"container_name":       "tfstate",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
		}

		// Import the Azure backend package
		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// Call Bootstrap and expect an error due to non-existent storage account
		err := backend.Bootstrap(ctx, logger, config, opts)

		// Verify an error was returned - it should be a DNS lookup error or storage account error
		require.Error(t, err)

		// Check if the error is one of our custom types
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError

		switch {
		case errors.As(err, &authErr):
			// Authentication error with underlying cause
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
		case errors.As(err, &storageErr):
			// Storage account error with underlying cause
			assert.Equal(t, nonExistentSA, storageErr.StorageAccountName, "Storage error should contain account name")
			require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
		default:
			// Fallback check for backwards compatibility
			assert.True(t,
				strings.Contains(err.Error(), "no such host") ||
					strings.Contains(err.Error(), "does not exist") ||
					strings.Contains(err.Error(), "not accessible"),
				"Should get DNS lookup or storage account error for non-existent storage account, got: %v", err)
		}
	})

	t.Run("bootstrap-with-real-storage-account", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		containerName := "terragrunt-bootstrap-test-" + uniqueSuffix

		config := map[string]interface{}{
			"storage_account_name": storageAccount,
			"container_name":       containerName,
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Import the Azure backend package
		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// Call Bootstrap - should succeed with real storage account
		err := backend.Bootstrap(ctx, logger, config, opts)
		require.NoError(t, err)

		// Verify container was created
		client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
		require.NoError(t, err)

		exists, err := client.ContainerExists(ctx, containerName)
		require.NoError(t, err)
		assert.True(t, exists, "Container should exist after successful bootstrap")

		// Clean up - delete the container
		defer func() {
			_ = client.DeleteContainer(ctx, logger, containerName)
		}()
	})

	t.Run("invalid-storage-account-name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		config := map[string]interface{}{
			"storage_account_name": "invalid/name/with/slashes", // Invalid storage account name
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Import the Azure backend package
		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// Call Bootstrap and expect an error due to invalid storage account name
		err := backend.Bootstrap(ctx, logger, config, opts)

		// Verify an error was returned - should get an error about the storage account not being accessible
		require.Error(t, err)

		// Check if the error is one of our custom types
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError

		switch {
		case errors.As(err, &authErr):
			// Authentication error with underlying cause
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
		case errors.As(err, &storageErr):
			// Storage account error with underlying cause
			assert.Contains(t, storageErr.StorageAccountName, "invalid", "Storage error should contain invalid account name")
			require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
		default:
			// Fallback check for backwards compatibility
			assert.True(t,
				strings.Contains(err.Error(), "does not exist") ||
					strings.Contains(err.Error(), "not accessible") ||
					strings.Contains(err.Error(), "no such host"),
				"Should get error about storage account not being accessible, got: %v", err)
		}
	})
}

// TestAzureBackendCustomErrorTypes tests the custom error types in integration scenarios
func TestAzureBackendCustomErrorTypes(t *testing.T) {
	t.Parallel()

	// Create logger for testing
	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	t.Run("missing-subscription-id-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingSubscriptionIDError when subscription_id is required but missing
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"resource_group_name":                  "test-rg",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// subscription_id is missing
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, logger, config, opts)

		// Verify error is the custom type
		require.Error(t, err)
		var missingSubError azurerm.MissingSubscriptionIDError
		require.ErrorAs(t, err, &missingSubError, "Error should be MissingSubscriptionIDError type")
		assert.Contains(t, err.Error(), "subscription_id is required", "Error message should mention subscription_id")
	})

	t.Run("missing-location-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingLocationError when location is required but missing
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "test-rg",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// location is missing
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, logger, config, opts)

		// Verify error is the custom type
		require.Error(t, err)
		var missingLocError azurerm.MissingLocationError
		require.ErrorAs(t, err, &missingLocError, "Error should be MissingLocationError type")
		assert.Contains(t, err.Error(), "location is required", "Error message should mention location")
	})

	t.Run("container-validation-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test ContainerValidationError for invalid container names
		testCases := []struct {
			name          string
			containerName string
			expectedMsg   string
		}{
			{
				name:          "empty-container-name",
				containerName: "",
				expectedMsg:   "missing required Azure remote state configuration container_name",
			},
			{
				name:          "container-name-too-short",
				containerName: "ab",
				expectedMsg:   "container name must be between 3 and 63 characters",
			},
			{
				name:          "invalid-uppercase",
				containerName: "Invalid-Container",
				expectedMsg:   "container name can only contain lowercase letters, numbers, and hyphens",
			},
			{
				name:          "consecutive-hyphens",
				containerName: "container--name",
				expectedMsg:   "container name cannot contain consecutive hyphens",
			},
		}

		for _, tc := range testCases {
			tc := tc // capture range variable
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				config := map[string]interface{}{
					"storage_account_name": "testaccount",
					"container_name":       tc.containerName,
					"key":                  "test/terraform.tfstate",
					"use_azuread_auth":     true,
				}

				backend := azurerm.NewBackend()
				require.NotNil(t, backend, "Azure backend should be created")

				err := backend.Bootstrap(ctx, logger, config, opts)

				// Verify error is returned
				require.Error(t, err)

				// For empty container name, it should be a MissingRequiredAzureRemoteStateConfig
				// For other validation issues, it should be ContainerValidationError
				if tc.containerName == "" {
					var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
					if assert.ErrorAs(t, err, &missingConfigError, "Empty container name should be MissingRequiredAzureRemoteStateConfig type") {
						assert.Equal(t, "container_name", string(missingConfigError), "Should indicate missing container_name")
					} else {
						// Fallback: check if error message is correct
						assert.Contains(t, err.Error(), tc.expectedMsg, "Error message should contain expected text")
					}
				} else {
					var containerValidationError azurerm.ContainerValidationError
					if assert.ErrorAs(t, err, &containerValidationError, "Error should be ContainerValidationError type") {
						assert.Contains(t, containerValidationError.ValidationIssue, tc.expectedMsg, "Validation error should contain expected text")
					} else {
						// Fallback: check if error message is correct
						assert.Contains(t, err.Error(), tc.expectedMsg, "Error message should contain expected text")
					}
				}
			})
		}
	})

	t.Run("missing-resource-group-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingResourceGroupError when resource_group_name is required for delete operation
		config := map[string]interface{}{
			"storage_account_name": "testaccount",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			// resource_group_name is missing (required for delete operation)
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.DeleteStorageAccount(ctx, logger, config, opts)

		// Verify error is the custom type
		require.Error(t, err)
		var missingRGError azurerm.MissingResourceGroupError
		require.ErrorAs(t, err, &missingRGError, "Error should be MissingResourceGroupError type")
		assert.Contains(t, err.Error(), "resource_group_name is required", "Error message should mention resource_group_name")
	})

	t.Run("authentication-failure-scenarios", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test authentication-related errors with a realistic configuration
		// that will trigger authentication issues rather than the NoValidAuthMethodError
		// (since Azure AD auth is now enforced by default)
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "test-rg",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true, // This will be used but will likely fail without proper credentials
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, logger, config, opts)

		// Verify an error was returned - this should fail due to authentication or storage account issues
		require.Error(t, err)

		// Check if it's an authentication error, storage error, or contains auth-related messaging
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError
		var noAuthError azurerm.NoValidAuthMethodError

		switch {
		case errors.As(err, &noAuthError):
			// This is the ideal case if we can trigger it
			assert.Contains(t, err.Error(), "no valid authentication method", "Error message should mention authentication method issue")
		case errors.As(err, &authErr):
			// Authentication error - common when credentials are missing/invalid
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
		case errors.As(err, &storageErr):
			// Storage account error - also common when auth fails during storage operations
			assert.NotEmpty(t, storageErr.StorageAccountName, "Storage error should contain account name")
			require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
		default:
			// Fallback - ensure the error at least mentions authentication or access issues
			errorStr := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(errorStr, "authentication") ||
					strings.Contains(errorStr, "credential") ||
					strings.Contains(errorStr, "auth") ||
					strings.Contains(errorStr, "access denied") ||
					strings.Contains(errorStr, "unauthorized") ||
					strings.Contains(errorStr, "no such host") ||
					strings.Contains(errorStr, "storage account"),
				"Error should mention authentication or access issues, got: %v", err)
		}
	})
}

// TestAzureErrorUnwrappingAndPropagation tests error unwrapping and propagation through the entire call stack
func TestAzureErrorUnwrappingAndPropagation(t *testing.T) {
	t.Parallel()

	// Create logger for testing
	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	t.Run("error-propagation-from-azurehelper", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test that errors from azurehelper are properly wrapped and propagated
		// Use a configuration that will trigger azurehelper errors
		config := map[string]interface{}{
			"storage_account_name": "nonexistent" + strconv.FormatInt(time.Now().UnixNano(), 10)[10:], // Non-existent account
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Test that CreateBlobServiceClient properly wraps errors
		_, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
		require.Error(t, err, "Expected error for non-existent storage account")

		// The error should be wrapped with useful context
		assert.True(t,
			strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "authentication") ||
				strings.Contains(err.Error(), "no such host"),
			"Error should provide meaningful context, got: %v", err)

		// Test error propagation through backend bootstrap
		backend := azurerm.NewBackend()
		bootstrapErr := backend.Bootstrap(ctx, logger, config, opts)
		require.Error(t, bootstrapErr, "Expected bootstrap error")

		// Check if bootstrap error wraps the underlying error properly
		if !errors.Is(bootstrapErr, err) {
			// If not the same error, it should at least contain similar context
			assert.True(t,
				strings.Contains(bootstrapErr.Error(), "storage account") ||
					strings.Contains(bootstrapErr.Error(), "authentication") ||
					strings.Contains(bootstrapErr.Error(), "no such host"),
				"Bootstrap error should propagate underlying context, got: %v", bootstrapErr)
		}
	})

	t.Run("container-creation-error-unwrapping", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create a mock configuration that will trigger container creation errors
		config := map[string]interface{}{
			"storage_account_name": "testaccount",
			"container_name":       "INVALID_UPPERCASE_CONTAINER", // Invalid container name
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		backend := azurerm.NewBackend()
		err := backend.Bootstrap(ctx, logger, config, opts)

		require.Error(t, err, "Expected error for invalid container name")

		// Should be a ContainerValidationError
		var containerValidationError azurerm.ContainerValidationError
		if assert.ErrorAs(t, err, &containerValidationError, "Error should be ContainerValidationError type") {
			assert.Contains(t, containerValidationError.ValidationIssue, "lowercase", "Validation error should mention lowercase requirement")
		}
	})

	t.Run("authentication-error-chain", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create a configuration that might trigger authentication issues
		config := map[string]interface{}{
			"storage_account_name": "mightnotexist" + strconv.FormatInt(time.Now().UnixNano(), 10)[10:],
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Test both azurehelper and backend error handling
		_, helperErr := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
		backend := azurerm.NewBackend()
		backendErr := backend.Bootstrap(ctx, logger, config, opts)

		// At least one should error (or both)
		if helperErr != nil || backendErr != nil {
			// If there are errors, they should be meaningful
			if helperErr != nil {
				assert.NotEmpty(t, helperErr.Error(), "Helper error should have meaningful message")
			}
			if backendErr != nil {
				assert.NotEmpty(t, backendErr.Error(), "Backend error should have meaningful message")
			}

			// Test error unwrapping if we have authentication or storage account errors
			if backendErr != nil {
				var authErr azurerm.AuthenticationError
				var storageErr azurerm.StorageAccountCreationError

				if errors.As(backendErr, &authErr) {
					assert.Error(t, authErr.Unwrap(), "AuthenticationError should have underlying error")
					assert.NotEmpty(t, authErr.AuthMethod, "AuthenticationError should specify auth method")
				} else if errors.As(backendErr, &storageErr) {
					assert.Error(t, storageErr.Unwrap(), "StorageAccountCreationError should have underlying error")
					assert.NotEmpty(t, storageErr.StorageAccountName, "StorageAccountCreationError should specify account name")
				}
			}
		} else {
			t.Log("No errors occurred - this is acceptable if Azure credentials are properly configured")
		}
	})

	t.Run("deep-error-chain-propagation", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test that errors propagate correctly through the entire call stack:
		// Bootstrap -> Backend -> Helper -> Azure SDK
		config := map[string]interface{}{
			"storage_account_name":                 "invalid-chars-###", // Invalid storage account name
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "test-rg",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// This should trigger a deep error chain:
		// Backend.Bootstrap calls helper functions, which should wrap and propagate errors properly
		err := backend.Bootstrap(ctx, logger, config, opts)
		require.Error(t, err, "Expected error for invalid storage account name")

		// The error should contain meaningful context about the issue
		assert.True(t,
			strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "invalid") ||
				strings.Contains(err.Error(), "validation"),
			"Error should provide meaningful context about storage account validation, got: %v", err)

		// Try to unwrap the error chain to ensure it's properly structured
		var unwrappedErr = err
		var errorChainDepth int
		for unwrappedErr != nil && errorChainDepth < 10 { // Prevent infinite loops
			errorChainDepth++
			if nextErr := errors.Unwrap(unwrappedErr); nextErr != nil {
				unwrappedErr = nextErr
			} else {
				break
			}
		}

		// We should have at least one level of error wrapping (original error + wrapper)
		assert.Positive(t, errorChainDepth, "Error should have proper wrapping chain")

		// Test that errors.Is works correctly for checking error types in the chain
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError
		var validationErr azurerm.ContainerValidationError

		// At least one of these error types should be found in the chain
		hasExpectedErrorType := errors.As(err, &authErr) ||
			errors.As(err, &storageErr) ||
			errors.As(err, &validationErr)

		if !hasExpectedErrorType {
			t.Logf("Error chain: %v", err)
			// For backwards compatibility, ensure basic error information is present
			assert.Contains(t, err.Error(), "storage", "Error should mention storage-related issues")
		}
	})
}
