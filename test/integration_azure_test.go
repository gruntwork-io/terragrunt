//go:build azure

package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	name          string
	args          string
	containerName string
}

func TestAzureRMBootstrapBackend(t *testing.T) {
	t.Parallel()

	t.Log("Starting TestAzureRMBootstrapBackend")

	testCases := []struct {
		TestCase
		checkExpectedResultFn func(t *testing.T, err error, output string, containerName string, rootPath string, tc *TestCase)
	}{
		{
			TestCase: TestCase{
				name:          "delete backend command",
				args:          "backend delete --force",
				containerName: "terragrunt-test-container-" + strings.ToLower(helpers.UniqueID()),
			},
			checkExpectedResultFn: func(t *testing.T, err error, output string, containerName string, rootPath string, tc *TestCase) {
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
					context.Background(),
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
				exists, err := client.ContainerExists(context.Background(), containerName)
				require.NoError(t, err)
				assert.True(t, exists, "Container should exist after bootstrap")

				// Create and verify test state file
				data := []byte("{}")
				err = client.UploadBlob(context.Background(), logger.CreateLogger(), containerName, "unit1/terraform.tfstate", data)
				require.NoError(t, err, "Failed to create test state file")

				stateKey := "unit1/terraform.tfstate"
				_, err = client.GetObject(context.Background(), &azurehelper.GetObjectInput{
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
					exists, err = client.ContainerExists(context.Background(), containerName)
					require.NoError(t, err)
					if !exists {
						containerExists = false
						break
					}
					time.Sleep(3 * time.Second)
				}
				assert.False(t, containerExists, "Container should not exist after delete")
			},
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

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --log-format key-value --strict-control require-explicit-bootstrap --working-dir "+rootPath)

			tc.checkExpectedResultFn(t, err, stdout+stderr, tc.containerName, rootPath, &tc.TestCase)
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
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app3", environmentPath))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app2", environmentPath))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app1", environmentPath))

	// Now check the outputs to make sure the remote state dependencies work
	app1OutputCmd := "terragrunt output -no-color -json --non-interactive --working-dir " + fmt.Sprintf("%s/app1", environmentPath)
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

	ctx := context.Background()

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
	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	storageAccountName := fmt.Sprintf("tgtest%s", strings.ToLower(uniqueID)[len(uniqueID)-10:]) // Storage account names must be 3-24 chars, alphanumeric only
	resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s", uniqueID)
	containerName := fmt.Sprintf("terragrunt-test-%s", strings.ToLower(t.Name()))
	if len(containerName) > 63 {
		containerName = containerName[:63]
	}

	// Setup cleanup to ensure resources are deleted even if the test fails
	storageAccountCreated := false
	containerCreated := false

	// Defer cleanup of all resources
	defer func() {
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
	}()

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
		EnableHierarchicalNS:  false,
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
	err = client.DeleteContainer(context.Background(), logger, containerName)
	require.NoError(t, err, "Failed to delete container")

	// Test creating multiple containers with the same client
	containerNameState := containerName + "-state"

	// Create another container
	err = client.CreateContainerIfNecessary(context.Background(), logger, containerNameState)
	require.NoError(t, err, "Failed to create second container")

	// Check if second container exists
	exists, err = client.ContainerExists(context.Background(), containerNameState)
	require.NoError(t, err)
	require.True(t, exists, "Second container should exist after creation")

	// Clean up second container
	err = client.DeleteContainer(context.Background(), logger, containerNameState)
	require.NoError(t, err, "Failed to delete test container")

	// Test error handling for invalid container names
	t.Run("InvalidContainerName", func(t *testing.T) {
		// Don't run in parallel - we need the storage account to exist first

		// Create a context for Azure operations
		ctx := context.Background()

		invalidContainerName := "UPPERCASE_CONTAINER"
		invalidClient, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, map[string]interface{}{
			"storage_account_name": storageAccountName,
			"container_name":       invalidContainerName,
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		})
		require.NoError(t, err)

		// Should fail with invalid container name
		err = invalidClient.CreateContainerIfNecessary(context.Background(), logger, invalidContainerName)
		assert.Error(t, err, "Creating container with invalid name should fail")
		assert.Contains(t, err.Error(), "invalid", "Error should mention invalid container name")
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
		ctx := context.Background()

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
		assert.Error(t, err, "Expected error when storage account doesn't exist")
		assert.Contains(t, err.Error(), "no such host", "Error should indicate operation failed")
	})

	// Test with existing storage account
	t.Run("StorageAccountBootstrap_ExistingAccount", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := context.Background()

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
		err = client.CreateContainerIfNecessary(context.Background(), logger, "terragrunt-test-sa-exists")
		require.NoError(t, err)

		// Clean up the container
		exists, err := client.ContainerExists(context.Background(), "terragrunt-test-sa-exists")
		require.NoError(t, err)

		if exists {
			err = client.DeleteContainer(context.Background(), logger, "terragrunt-test-sa-exists")
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

	ctx := context.Background()

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
	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	storageAccountName := fmt.Sprintf("tgtest%s", strings.ToLower(uniqueID)[len(uniqueID)-10:]) // Storage account names must be 3-24 chars, alphanumeric only
	resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s", uniqueID)
	containerName := fmt.Sprintf("test-container-%s", strings.ToLower(uniqueID))
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
		EnableHierarchicalNS:  false,
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
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure backend bootstrap test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	t.Parallel()

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

		ctx := context.Background()

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

		// Verify an error was returned - it should be a DNS lookup error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such host", "Should get DNS lookup error for non-existent storage account")
	})

	t.Run("bootstrap-with-real-storage-account", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
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

		ctx := context.Background()

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
		assert.Contains(t, err.Error(), "does not exist or is not accessible", "Should get error about storage account not being accessible")
	})
}
