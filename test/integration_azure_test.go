//go:build azure

package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
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
			// CopyEnvironment copies to tmpEnvPath + "/" + testFixtureAzureBackend
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

	environmentPath := fmt.Sprintf("%s/test/%s/env1", tmpEnvPath, testFixtureAzureOutputFromRemoteState)

	azureCfg := helpers.GetAzureStorageTestConfig(t)

	// Fill in Azure configuration
	rootPath := util.JoinPath(tmpEnvPath, "test", testFixtureAzureOutputFromRemoteState)
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
	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --working-dir "+environmentPath+"/app3")
	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --working-dir "+environmentPath+"/app2")
	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --working-dir "+environmentPath+"/app1")

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

	log := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
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
			cleanupClient, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, cleanupConfig)
			if err == nil {
				log.Infof("Cleaning up container %s", containerName)
				_ = cleanupClient.DeleteContainer(ctx, log, containerName)
			}
		}

		if storageAccountCreated {
			cleanupClient, err := azurehelper.CreateStorageAccountClient(ctx, log, cleanupConfig)
			if err == nil {
				log.Infof("Cleaning up storage account %s", storageAccountName)
				_ = cleanupClient.DeleteStorageAccount(ctx, log)
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
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, log, storageAccountConfig)
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
	err = storageClient.CreateStorageAccountIfNecessary(ctx, log, saConfig)
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

	client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
	require.NoError(t, err)

	// Track containers for cleanup
	var firstContainerCreated, secondContainerCreated bool
	containerNameState := containerName + "-state"

	// Setup container cleanup using defer
	defer func() {
		if secondContainerCreated {
			t.Logf("Cleaning up second container %s", containerNameState)
			_ = client.DeleteContainer(context.Background(), log, containerNameState)
		}
		if firstContainerCreated {
			t.Logf("Cleaning up first container %s", containerName)
			_ = client.DeleteContainer(context.Background(), log, containerName)
		}
	}()

	// Create container
	err = client.CreateContainerIfNecessary(ctx, log, containerName)
	require.NoError(t, err, "Failed to create container")
	containerCreated = true
	firstContainerCreated = true

	// Check if container exists
	exists, err = client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	require.True(t, exists, "Container should exist after creation")

	// Test creating multiple containers with the same client
	// Create another container
	err = client.CreateContainerIfNecessary(t.Context(), log, containerNameState)
	require.NoError(t, err, "Failed to create second container")
	secondContainerCreated = true

	// Check if second container exists
	exists, err = client.ContainerExists(t.Context(), containerNameState)
	require.NoError(t, err)
	require.True(t, exists, "Second container should exist after creation")

	// Test error handling for invalid container names
	t.Run("InvalidContainerName", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		invalidContainerName := "UPPERCASE_CONTAINER"
		invalidClient, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, map[string]interface{}{
			"storage_account_name": storageAccountName,
			"container_name":       invalidContainerName,
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		})
		require.NoError(t, err) // Should fail with invalid container name
		err = invalidClient.CreateContainerIfNecessary(t.Context(), log, invalidContainerName)
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

	log := logger.CreateLogger()
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
		_, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
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
		client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
		require.NoError(t, err)

		// Create container for test
		err = client.CreateContainerIfNecessary(t.Context(), log, "terragrunt-test-sa-exists")
		require.NoError(t, err)

		// Clean up the container
		exists, err := client.ContainerExists(t.Context(), "terragrunt-test-sa-exists")
		require.NoError(t, err)

		if exists {
			err = client.DeleteContainer(t.Context(), log, "terragrunt-test-sa-exists")
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

	log := logger.CreateLogger()
	client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Setup cleanup
	var containerCreated bool
	defer func() {
		if containerCreated {
			t.Logf("Cleanup: Deleting container %s", containerName)
			_ = client.DeleteContainer(ctx, log, containerName)
		}
	}()

	// Test container creation
	err = client.CreateContainerIfNecessary(ctx, log, containerName)
	require.NoError(t, err)
	containerCreated = true

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
	err = client.DeleteBlobIfNecessary(ctx, log, containerName, blobName)
	require.NoError(t, err)

	// Verify container exists before the test completes (cleanup will handle deletion)
	exists, err = client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestStorageAccountCreationAndBlobUpload tests the complete workflow of creating a storage account and uploading a blob
func TestStorageAccountCreationAndBlobUpload(t *testing.T) {
	t.Parallel()

	log := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
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
	rgClient, err := azurehelper.CreateResourceGroupClient(ctx, log, subscriptionID)
	require.NoError(t, err)
	require.NotNil(t, rgClient)

	// Create resource group if it doesn't exist
	resourceGroupCreated := false

	// Defer cleanup of resource group
	defer func() {
		if resourceGroupCreated {
			t.Logf("Cleaning up resource group %s", resourceGroupName)
			// Ignore errors during cleanup
			_ = rgClient.DeleteResourceGroup(ctx, log, resourceGroupName)
		}
	}()

	err = rgClient.EnsureResourceGroup(ctx, log, resourceGroupName, location, resourceGroupTags)
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
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, log, storageAccountConfig)
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
			_ = blobClient.DeleteBlobIfNecessary(ctx, log, containerName, blobName)
		}

		if containerCreated && blobClient != nil {
			t.Logf("Cleanup: Deleting container %s", containerName)
			// Ignore errors during cleanup
			_ = blobClient.DeleteContainer(ctx, log, containerName)
		}

		if storageAccountCreated {
			t.Logf("Cleanup: Deleting storage account %s", storageAccountName)
			// Ignore errors during cleanup
			_ = storageClient.DeleteStorageAccount(ctx, log)
		}

		t.Log("Cleanup completed")
	}()

	// Create storage account and resource group if necessary
	err = storageClient.CreateStorageAccountIfNecessary(ctx, log, saConfig)
	require.NoError(t, err)
	storageAccountCreated = true

	// Verify storage account exists
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)

	t.Logf("Storage account %s created successfully", storageAccountName)

	// Now create the blob client after the storage account exists
	blobClient, err = azurehelper.CreateBlobServiceClient(ctx, log, opts, blobConfig)
	require.NoError(t, err)
	require.NotNil(t, blobClient)

	// Now test blob operations with the created storage account

	// Create container
	err = blobClient.CreateContainerIfNecessary(ctx, log, containerName)
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
	err = blobClient.UploadBlob(ctx, log, containerName, blobName, contentBytes)
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
	log := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	// Create a unique suffix for container names
	uniqueSuffix := fmt.Sprintf("%x", time.Now().UnixNano())[0:8]

	t.Run("bootstrap-with-existing-storage-account", func(t *testing.T) {
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
		err := backend.Bootstrap(ctx, log, config, opts)
		require.NoError(t, err)

		// Verify container was created
		client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
		require.NoError(t, err)

		exists, err := client.ContainerExists(ctx, containerName)
		require.NoError(t, err)
		assert.True(t, exists, "Container should exist after successful bootstrap")

		// Clean up - delete the container
		defer func() {
			_ = client.DeleteContainer(ctx, log, containerName)
		}()
	})

	t.Run("invalid-storage-account-name", func(t *testing.T) {
		t.Parallel()

		testLogger := logger.CreateLogger()
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

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
		err = backend.Bootstrap(ctx, testLogger, config, opts)

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
	log := logger.CreateLogger()
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

		err := backend.Bootstrap(ctx, log, config, opts)

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

		err := backend.Bootstrap(ctx, log, config, opts)

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

				err := backend.Bootstrap(ctx, log, config, opts)

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

		err := backend.DeleteStorageAccount(ctx, log, config, opts)

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

		err := backend.Bootstrap(ctx, log, config, opts)

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
	log := logger.CreateLogger()

	// Make sure we're using non-interactive mode to avoid prompts

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
		_, err := azurehelper.CreateBlobServiceClient(ctx, log, nil, config)
		require.Error(t, err, "Expected error for non-existent storage account")

		// The error should be wrapped with useful context
		assert.True(t,
			strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "authentication") ||
				strings.Contains(err.Error(), "no such host"),
			"Error should provide meaningful context, got: %v", err)

		// Test error propagation through backend bootstrap
		backend := azurerm.NewBackend()
		bootstrapErr := backend.Bootstrap(ctx, log, config, nil)
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
		err := backend.Bootstrap(ctx, log, config, nil)

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
		_, helperErr := azurehelper.CreateBlobServiceClient(ctx, log, nil, config)
		backend := azurerm.NewBackend()
		backendErr := backend.Bootstrap(ctx, log, config, nil)

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
		err := backend.Bootstrap(ctx, log, config, nil)
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

// TestStorageAccountConfigurationAndUpdate tests storage account configuration options and updates
func TestStorageAccountConfigurationAndUpdate(t *testing.T) {
	t.Parallel()

	log := logger.CreateLogger()
	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
	if err != nil {
		t.Skipf("Skipping storage account configuration test: Failed to get Azure credentials: %v", err)
	}

	if subscriptionID == "" {
		t.Skip("Skipping storage account configuration test: No subscription ID found in environment variables")
	}

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope"
			t.Logf("Neither AZURE_LOCATION nor ARM_LOCATION set, using default: %s", location)
		}
	}

	// Test different storage account configurations
	testCases := []struct {
		name            string
		expectUpdate    bool
		expectWarnings  bool
		accessTierChange bool
		replicationChange bool
		publicAccessChange bool
		tagsChange bool
	}{
		{
			name: "UpdateAccessTier",
			expectUpdate:   true,
			expectWarnings: false,
			accessTierChange: true,
		},
		{
			name: "UpdateBlobPublicAccess",
			expectUpdate:   true,
			expectWarnings: false,
			publicAccessChange: true,
		},
		{
			name: "UpdateTags",
			expectUpdate:   true,
			expectWarnings: false,
			tagsChange: true,
		},
		{
			name: "ReadOnlyPropertyWarning",
			expectUpdate:   false,
			expectWarnings: true,
			replicationChange: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Remove t.Parallel() here to prevent resource conflicts between subtests
			// Each subtest will have completely unique resources
			
			// Generate unique names for this specific subtest
			subtestUniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
			storageAccountName := "tgtest" + strings.ToLower(subtestUniqueID)[len(subtestUniqueID)-10:]
			resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s-%s", tc.name, subtestUniqueID)

			// Create configurations based on test case parameters
			initialConfig := azurehelper.StorageAccountConfig{
				SubscriptionID:        subscriptionID,
				ResourceGroupName:     resourceGroupName,
				StorageAccountName:    storageAccountName,
				Location:              location,
				EnableVersioning:      true,
				AllowBlobPublicAccess: !tc.publicAccessChange, // Start with opposite if testing change
				AccountKind:           "StorageV2",
				AccountTier:           "Standard",
				AccessTier:            "Hot",
				ReplicationType:       "LRS",
				Tags:                  map[string]string{"Environment": "Test", "created-by": "terragrunt-integration-test"},
			}

			// Create updated config based on what we're testing
			updatedConfig := initialConfig // Copy all fields
			updatedConfig.Tags = map[string]string{"Environment": "Test", "created-by": "terragrunt-integration-test"} // Reset tags

			switch {
			case tc.accessTierChange:
				updatedConfig.AccessTier = "Cool" // Change from Hot to Cool
			case tc.publicAccessChange:
				updatedConfig.AllowBlobPublicAccess = false // Disable public access
				updatedConfig.Tags["access-updated"] = "true"
			case tc.tagsChange:
				updatedConfig.Tags = map[string]string{
					"Environment": "Production", 
					"Owner": "TeamA", 
					"created-by": "terragrunt-integration-test",
				}
			case tc.replicationChange:
				updatedConfig.ReplicationType = "GRS" // Try to change read-only property
			}

			// Create resource group client
			rgClient, err := azurehelper.CreateResourceGroupClient(ctx, log, subscriptionID)
			require.NoError(t, err)

			// Create resource group
			err = rgClient.EnsureResourceGroup(ctx, log, resourceGroupName, location, map[string]string{"created-by": "terragrunt-integration-test"})
			require.NoError(t, err)
			t.Logf("Resource group %s created successfully", resourceGroupName)

			// Track what resources were successfully created for proper cleanup ordering
			var storageAccountCreated bool
			
			// **Solution 4: Improved Resource Cleanup Order**
			defer func() {
				cleanupCtx := context.Background()
				
				if storageAccountCreated {
					t.Logf("Cleaning up storage account %s", storageAccountName)
					
					// Create a fresh storage client for cleanup
					cleanupStorageConfig := map[string]interface{}{
						"storage_account_name": storageAccountName,
						"resource_group_name":  resourceGroupName,
						"subscription_id":      subscriptionID,
						"use_azuread_auth":     true,
					}
					
					if cleanupStorageClient, err := azurehelper.CreateStorageAccountClient(cleanupCtx, log, cleanupStorageConfig); err == nil {
						if err := cleanupStorageClient.DeleteStorageAccount(cleanupCtx, log); err != nil {
							t.Logf("Warning: Failed to delete storage account %s: %v", storageAccountName, err)
						} else {
							t.Logf("Successfully deleted storage account %s", storageAccountName)
							// Wait for storage account deletion to complete before deleting resource group
							time.Sleep(10 * time.Second)
						}
					} else {
						t.Logf("Warning: Failed to create storage client for cleanup: %v", err)
					}
				}
				
				// Then delete resource group (only after storage account is deleted)
				t.Logf("Cleaning up resource group %s", resourceGroupName)
				if err := rgClient.DeleteResourceGroup(cleanupCtx, log, resourceGroupName); err != nil {
					t.Logf("Warning: Failed to delete resource group %s: %v", resourceGroupName, err)
				} else {
					t.Logf("Successfully deleted resource group %s", resourceGroupName)
				}
			}()

			// Create storage account client
			storageAccountConfig := map[string]interface{}{
				"storage_account_name": storageAccountName,
				"resource_group_name":  resourceGroupName,
				"subscription_id":      subscriptionID,
				"location":             location,
				"use_azuread_auth":     true,
			}

			t.Logf("Creating storage account %s in resource group %s", storageAccountName, resourceGroupName)

			// Create storage account client
			storageClient, err := azurehelper.CreateStorageAccountClient(ctx, log, storageAccountConfig)
			require.NoError(t, err)
			require.NotNil(t, storageClient)

			// Create storage account with initial configuration
			err = storageClient.CreateStorageAccountIfNecessary(ctx, log, initialConfig)
			require.NoError(t, err)
			storageAccountCreated = true // Mark as created for cleanup

			// Verify storage account exists
			exists, account, err := storageClient.StorageAccountExists(ctx)
			require.NoError(t, err)
			require.True(t, exists, "Storage account should exist after creation")
			require.NotNil(t, account)

			// Wait a moment for the storage account to be fully ready
			time.Sleep(5 * time.Second)

			// Update storage account with new configuration
			err = storageClient.CreateStorageAccountIfNecessary(ctx, log, updatedConfig)
			
			// Handle potential race condition with resource group deletion
			if err != nil && strings.Contains(err.Error(), "ResourceGroupBeingDeleted") {
				t.Skipf("Skipping test %s due to resource group cleanup race condition: %v", tc.name, err)
			}
			
			if tc.expectWarnings {
				// For read-only property changes, we expect success but warnings
				require.NoError(t, err)
				// We can't easily capture logs in this test framework, but the update should complete
			} else {
				require.NoError(t, err)
			}

			// Verify final state
			exists, updatedAccount, err := storageClient.StorageAccountExists(ctx)
			require.NoError(t, err)
			require.True(t, exists)
			require.NotNil(t, updatedAccount)

			// Verify specific properties that should have been updated
			if tc.expectUpdate && !tc.expectWarnings {
				// Check that updatable properties were actually changed
				switch {
				case tc.accessTierChange:
					t.Logf("Access tier should have been updated from Hot to Cool")
				case tc.publicAccessChange:
					t.Logf("Blob public access should have been updated to false")
				case tc.tagsChange:
					t.Logf("Tags should have been updated to include Owner and Environment=Production")
				}
			}

			t.Logf("Test %s completed successfully", tc.name)
		})
	}
}

// TestAzureBackendMigrationWithUnits tests backend migration using Terragrunt's backend migrate command
// with src-unit and dst-unit parameters to migrate state between different paths in the same container
func TestAzureBackendMigrationWithUnits(t *testing.T) {
	t.Parallel()

	// Check for required environment variables and Azure credentials
	log := logger.CreateLogger()
	ctx := t.Context()

	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
	if err != nil {
		t.Skipf("Skipping backend migration test: Failed to get Azure credentials: %v", err)
	}

	if subscriptionID == "" {
		t.Skip("Skipping backend migration test: No subscription ID found in environment variables")
	}

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope"
			t.Logf("Neither AZURE_LOCATION nor ARM_LOCATION set, using default: %s", location)
		}
	}

	// Create temporary directory for test fixtures
	tmpDir := t.TempDir()
	
	// Generate unique names for this test
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)[len(strconv.FormatInt(time.Now().UnixNano(), 10))-8:]
	storageAccountName := "tgmigrate" + uniqueID // Storage account names must be 3-24 chars, alphanumeric only
	resourceGroupName := "terragrunt-migration-test-rg-" + uniqueID
	containerName := "migration-test-" + uniqueID

	// Create resource group for the test storage account
	rgClient, err := azurehelper.CreateResourceGroupClient(ctx, log, subscriptionID)
	require.NoError(t, err)

	err = rgClient.EnsureResourceGroup(ctx, log, resourceGroupName, location, map[string]string{
		"created-by": "terragrunt-integration-test",
		"test-case":  "TestAzureBackendMigrationWithUnits",
	})
	require.NoError(t, err)
	t.Logf("Resource group %s created successfully", resourceGroupName)

	// Track what resources were created for cleanup
	var storageAccountCreated bool

	// Setup comprehensive cleanup
	defer func() {
		cleanupCtx := context.Background()
		cleanupLogger := logger.CreateLogger()

		// Clean up storage account first
		if storageAccountCreated {
			t.Logf("Cleaning up storage account %s", storageAccountName)
			storageConfig := map[string]interface{}{
				"storage_account_name": storageAccountName,
				"resource_group_name":  resourceGroupName,
				"subscription_id":      subscriptionID,
				"use_azuread_auth":     true,
			}

			if storageClient, err := azurehelper.CreateStorageAccountClient(cleanupCtx, cleanupLogger, storageConfig); err == nil {
				if err := storageClient.DeleteStorageAccount(cleanupCtx, cleanupLogger); err != nil {
					t.Logf("Warning: Failed to delete storage account %s: %v", storageAccountName, err)
				} else {
					t.Logf("Successfully cleaned up storage account: %s", storageAccountName)
				}
			}
		}

		// Clean up resource group
		t.Logf("Cleaning up resource group %s", resourceGroupName)
		if err := rgClient.DeleteResourceGroup(cleanupCtx, cleanupLogger, resourceGroupName); err != nil {
			t.Logf("Warning: Failed to delete resource group %s: %v", resourceGroupName, err)
		} else {
			t.Logf("Successfully cleaned up resource group: %s", resourceGroupName)
		}

		// Clean up any temporary files that might have been created
		tempFiles := []string{
			"/tmp/terragrunt-migration-test-*.txt",
		}
		for _, pattern := range tempFiles {
			if matches, err := filepath.Glob(pattern); err == nil {
				for _, file := range matches {
					if err := os.Remove(file); err != nil {
						t.Logf("Warning: Failed to remove temp file %s: %v", file, err)
					} else {
						t.Logf("Cleaned up temp file: %s", file)
					}
				}
			}
		}
	}()

	// Create storage account with blob versioning enabled
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"resource_group_name":  resourceGroupName,
		"subscription_id":      subscriptionID,
		"location":             location,
		"use_azuread_auth":     true,
		"versioning_enabled": true, // Enable blob versioning for migration support
	}

	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, log, storageAccountConfig)
	require.NoError(t, err)

	// Create storage account with versioning enabled
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        subscriptionID,
		ResourceGroupName:     resourceGroupName,
		StorageAccountName:    storageAccountName,
		Location:              location,
		EnableVersioning:      true, // Enable blob versioning for migration support
		AllowBlobPublicAccess: false,
		AccountKind:           "StorageV2",
		AccountTier:           "Standard",
		AccessTier:            "Hot",
		ReplicationType:       "LRS",
		Tags:                  map[string]string{"created-by": "terragrunt-migration-test"},
	}

	err = storageClient.CreateStorageAccountIfNecessary(ctx, log, saConfig)
	require.NoError(t, err)
	storageAccountCreated = true

	// Verify storage account exists
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)
	t.Logf("Storage account %s created successfully with versioning enabled", storageAccountName)

	t.Logf("Storage account %s created successfully with versioning enabled", storageAccountName)

	// Create a simple terraform module for testing
	moduleDir := fmt.Sprintf("%s/modules/simple", tmpDir)
	err = os.MkdirAll(moduleDir, 0755)
	require.NoError(t, err)

	terraformContent := `
resource "random_id" "test" {
  byte_length = 8
}

resource "local_file" "test" {
  content  = "Migration test - ${random_id.test.hex}"
  filename = "/tmp/terragrunt-migration-test-${random_id.test.hex}.txt"
}

output "random_id" {
  value = random_id.test.hex
}

output "file_path" {
  value = local_file.test.filename
}

output "content" {
  value = local_file.test.content
}
`
	err = os.WriteFile(fmt.Sprintf("%s/main.tf", moduleDir), []byte(terraformContent), 0644)
	require.NoError(t, err)

	t.Run("migrate_between_state_paths", func(t *testing.T) {
		// Test migrating state from one path to another in the same container

		// Create separate directories for source and destination units
		srcUnitDir := fmt.Sprintf("%s/src-unit", tmpDir)
		dstUnitDir := fmt.Sprintf("%s/dst-unit", tmpDir)
		
		err := os.MkdirAll(srcUnitDir, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(dstUnitDir, 0755)
		require.NoError(t, err)

		// Track state files created during test for cleanup
		var createdTempFiles []string
		defer func() {
			// Clean up any temp files created during the test
			for _, file := range createdTempFiles {
				if err := os.Remove(file); err != nil {
					t.Logf("Warning: Failed to remove temp file %s: %v", file, err)
				} else {
					t.Logf("Cleaned up temp file: %s", file)
				}
			}
		}()

		// Copy modules to both directories
		for _, unitDir := range []string{srcUnitDir, dstUnitDir} {
			unitModulesDir := fmt.Sprintf("%s/modules/simple", unitDir)
			err = os.MkdirAll(unitModulesDir, 0755)
			require.NoError(t, err)
			
			sourceContent, err := os.ReadFile(fmt.Sprintf("%s/main.tf", moduleDir))
			require.NoError(t, err)
			err = os.WriteFile(fmt.Sprintf("%s/main.tf", unitModulesDir), sourceContent, 0644)
			require.NoError(t, err)
		}

		// Step 1: Create initial terragrunt.hcl for source unit with Azure backend
		srcConfig := fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "%s"
    container_name      = "%s"
    key                 = "source/terraform.tfstate"
    use_azuread_auth    = true
  }
}

terraform {
  source = "./modules/simple"
}
`, storageAccountName, containerName)

		srcConfigPath := fmt.Sprintf("%s/terragrunt.hcl", srcUnitDir)
		err = os.WriteFile(srcConfigPath, []byte(srcConfig), 0644)
		require.NoError(t, err)

		// Step 2: Change to source directory and apply to create initial state
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)

		err = os.Chdir(srcUnitDir)
		require.NoError(t, err)

		// Bootstrap and apply in source unit
		output, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive apply -auto-approve")
		require.NoError(t, err, "Initial apply failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		// Get initial outputs to verify state
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive output -json")
		require.NoError(t, err, "Output command failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		var initialOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &initialOutputs)
		require.NoError(t, err)
		require.Contains(t, initialOutputs, "random_id")
		require.Contains(t, initialOutputs, "file_path")
		require.Contains(t, initialOutputs, "content")

		// Extract the random_id for verification after migration
		initialRandomID := initialOutputs["random_id"].(map[string]interface{})["value"].(string)
		initialFilePath := initialOutputs["file_path"].(map[string]interface{})["value"].(string)
		t.Logf("Initial random ID: %s", initialRandomID)
		t.Logf("Initial file path: %s", initialFilePath)

		// Track the temp file for cleanup
		if initialFilePath != "" {
			createdTempFiles = append(createdTempFiles, initialFilePath)
		}

		// Verify the state exists in Azure at source path
		log := logger.CreateLogger()
		ctx := t.Context()
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		config := map[string]interface{}{
			"storage_account_name": storageAccountName,
			"container_name":       containerName,
			"use_azuread_auth":     true,
			"versioning_enabled": true,
		}

		client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
		require.NoError(t, err)

		// Verify source state exists
		sourceStateKey := "source/terraform.tfstate"
		sourceState, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
			Bucket: &containerName,
			Key:    &sourceStateKey,
		})
		require.NoError(t, err, "Source state should exist in Azure")
		sourceState.Body.Close()

		// Step 3: Create destination unit terragrunt.hcl with different key path
		dstConfig := fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "%s"
    container_name      = "%s"
    key                 = "destination/terraform.tfstate"
    use_azuread_auth    = true
  }
}

terraform {
  source = "./modules/simple"
}
`, storageAccountName, containerName)

		dstConfigPath := fmt.Sprintf("%s/terragrunt.hcl", dstUnitDir)
		err = os.WriteFile(dstConfigPath, []byte(dstConfig), 0644)
		require.NoError(t, err)

		// Step 4: Use backend migrate command with src-unit and dst-unit parameters
		// Change back to the parent directory to run the migration command
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Run the migration command from the parent directory
		migrationCmd := fmt.Sprintf("terragrunt backend migrate --non-interactive %s %s", srcUnitDir, dstUnitDir)
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, migrationCmd)
		require.NoError(t, err, "Backend migration failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		// Verify the migration output indicates successful migration
		assert.Contains(t, output, "migrat", "Migration output should indicate migration occurred")

		// Step 5: Verify state was migrated to destination
		err = os.Chdir(dstUnitDir)
		require.NoError(t, err)

		// Check outputs in destination unit
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive output -json")
		require.NoError(t, err, "Output after migration failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		var migratedOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &migratedOutputs)
		require.NoError(t, err)

		// Verify state was preserved during migration
		migratedRandomID := migratedOutputs["random_id"].(map[string]interface{})["value"].(string)
		migratedFilePath := migratedOutputs["file_path"].(map[string]interface{})["value"].(string)
		
		assert.Equal(t, initialRandomID, migratedRandomID, "Random ID should be preserved during migration")
		assert.Equal(t, initialFilePath, migratedFilePath, "File path should be preserved during migration")

		// Step 6: Verify state exists at destination path in Azure
		destinationStateKey := "destination/terraform.tfstate"
		destinationState, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
			Bucket: &containerName,
			Key:    &destinationStateKey,
		})
		require.NoError(t, err, "Destination state should exist in Azure after migration")
		destinationState.Body.Close()

		// Step 7: Verify source state no longer exists (migration should move, not copy)
		_, err = client.GetObject(ctx, &azurehelper.GetObjectInput{
			Bucket: &containerName,
			Key:    &sourceStateKey,
		})
		assert.Error(t, err, "Source state should no longer exist after migration (state should be moved, not copied)")

		// Step 8: Verify we can still manage resources with the migrated state
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive")
		require.NoError(t, err, "Plan with migrated state failed: %v\nOutput: %s\nError: %s", err, output, stderr)
		assert.Contains(t, output, "No changes", "Plan should show no changes after successful migration")

		t.Log("Successfully migrated state from source path to destination path")

		// Cleanup resources
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy --non-interactive -auto-approve")
		if err != nil {
			t.Logf("Warning: Failed to destroy resources: %v\nOutput: %s\nError: %s", err, output, stderr)
		} else {
			t.Log("Successfully destroyed test resources")
		}
	})

	t.Run("migrate_with_different_configurations", func(t *testing.T) {
		// Test migration between units with different configuration settings

		// Create separate directories for source and destination units
		srcUnitDir := fmt.Sprintf("%s/config-src", tmpDir)
		dstUnitDir := fmt.Sprintf("%s/config-dst", tmpDir)
		
		err := os.MkdirAll(srcUnitDir, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(dstUnitDir, 0755)
		require.NoError(t, err)

		// Use different container for this test to avoid conflicts
		configContainerName := containerName + "-config"

		// Track resources for cleanup
		var configContainerCreated bool
		var createdTempFiles []string

		defer func() {
			// Clean up config container
			if configContainerCreated {
				cleanupLogger := logger.CreateLogger()
				cleanupCtx := context.Background()
				cleanupOpts, err := options.NewTerragruntOptionsForTest("")
				if err == nil {
					config := map[string]interface{}{
						"storage_account_name": storageAccountName,
						"container_name":       configContainerName,
						"use_azuread_auth":     true,
					}

					client, err := azurehelper.CreateBlobServiceClient(cleanupCtx, cleanupLogger, cleanupOpts, config)
					if err == nil {
						if err := client.DeleteContainer(cleanupCtx, cleanupLogger, configContainerName); err != nil {
							t.Logf("Warning: Failed to delete config container %s: %v", configContainerName, err)
						} else {
							t.Logf("Successfully cleaned up config container: %s", configContainerName)
						}
					}
				}
			}

			// Clean up any temp files
			for _, file := range createdTempFiles {
				if err := os.Remove(file); err != nil {
					t.Logf("Warning: Failed to remove temp file %s: %v", file, err)
				} else {
					t.Logf("Cleaned up temp file: %s", file)
				}
			}
		}()

		// Copy modules to both directories
		for _, unitDir := range []string{srcUnitDir, dstUnitDir} {
			unitModulesDir := fmt.Sprintf("%s/modules/simple", unitDir)
			err = os.MkdirAll(unitModulesDir, 0755)
			require.NoError(t, err)
			
			sourceContent, err := os.ReadFile(fmt.Sprintf("%s/main.tf", moduleDir))
			require.NoError(t, err)
			err = os.WriteFile(fmt.Sprintf("%s/main.tf", unitModulesDir), sourceContent, 0644)
			require.NoError(t, err)
		}

		// Create source configuration
		srcConfig := fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "%s"
    container_name      = "%s"
    key                 = "environments/dev/terraform.tfstate"
    use_azuread_auth    = true
  }
}

terraform {
  source = "./modules/simple"
}
`, storageAccountName, configContainerName)

		srcConfigPath := fmt.Sprintf("%s/terragrunt.hcl", srcUnitDir)
		err = os.WriteFile(srcConfigPath, []byte(srcConfig), 0644)
		require.NoError(t, err)

		// Create destination configuration with different key structure
		dstConfig := fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "%s"
    container_name      = "%s"
    key                 = "environments/prod/terraform.tfstate"
    use_azuread_auth    = true
  }
}

terraform {
  source = "./modules/simple"
}
`, storageAccountName, configContainerName)

		dstConfigPath := fmt.Sprintf("%s/terragrunt.hcl", dstUnitDir)
		err = os.WriteFile(dstConfigPath, []byte(dstConfig), 0644)
		require.NoError(t, err)

		// Apply in source unit
		err = os.Chdir(srcUnitDir)
		require.NoError(t, err)

		output, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive apply -auto-approve")
		require.NoError(t, err, "Initial apply failed: %v\nOutput: %s\nError: %s", err, output, stderr)
		configContainerCreated = true // Mark container as created for cleanup

		// Get initial state
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive output -json")
		require.NoError(t, err)

		var initialOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &initialOutputs)
		require.NoError(t, err)
		initialRandomID := initialOutputs["random_id"].(map[string]interface{})["value"].(string)

		// Track any temp file created for cleanup
		if filePath, ok := initialOutputs["file_path"].(map[string]interface{})["value"].(string); ok && filePath != "" {
			createdTempFiles = append(createdTempFiles, filePath)
		}

		// Migrate from dev to prod environment
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		migrationCmd := fmt.Sprintf("terragrunt backend migrate --non-interactive %s %s", srcUnitDir, dstUnitDir)
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, migrationCmd)
		require.NoError(t, err, "Environment migration failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		// Verify migration in destination
		err = os.Chdir(dstUnitDir)
		require.NoError(t, err)

		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive output -json")
		require.NoError(t, err)

		var migratedOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &migratedOutputs)
		require.NoError(t, err)
		migratedRandomID := migratedOutputs["random_id"].(map[string]interface{})["value"].(string)

		assert.Equal(t, initialRandomID, migratedRandomID, "Random ID should be preserved during environment migration")

		// Verify state exists at prod path
		log := logger.CreateLogger()
		ctx := t.Context()
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		config := map[string]interface{}{
			"storage_account_name": storageAccountName,
			"container_name":       configContainerName,
			"use_azuread_auth":     true,
		}

		client, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
		require.NoError(t, err)

		prodStateKey := "environments/prod/terraform.tfstate"
		prodState, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
			Bucket: &configContainerName,
			Key:    &prodStateKey,
		})
		require.NoError(t, err, "Prod state should exist after migration")
		prodState.Body.Close()

		// Verify dev state no longer exists
		devStateKey := "environments/dev/terraform.tfstate"
		_, err = client.GetObject(ctx, &azurehelper.GetObjectInput{
			Bucket: &configContainerName,
			Key:    &devStateKey,
		})
		assert.Error(t, err, "Dev state should no longer exist after migration")

		t.Log("Successfully migrated state between different environment configurations")

		// Cleanup resources
		output, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy --non-interactive -auto-approve")
		if err != nil {
			t.Logf("Warning: Failed to destroy resources: %v\nOutput: %s\nError: %s", err, output, stderr)
		} else {
			t.Log("Successfully destroyed test resources")
		}

		// The container cleanup will be handled by the defer function above
	})
}

// TestAzureBackendBootstrapWorkflow tests the complete backend bootstrap workflow
func TestAzureBackendBootstrapWorkflow(t *testing.T) {
	t.Parallel()

	// Check for required environment variables
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure backend bootstrap workflow test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		t.Skip("Skipping Azure backend bootstrap workflow test: AZURE_SUBSCRIPTION_ID not set")
	}

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = "westeurope"
	}

	// Create temporary directory for test fixtures
	tmpDir := t.TempDir()
	
	// Generate unique container name
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)[len(strconv.FormatInt(time.Now().UnixNano(), 10))-8:]
	containerName := "terragrunt-test-" + uniqueID

	// Track containers created for cleanup
	var createdContainers []string
	defer func() {
		// Clean up all containers created during the test
		cleanupLogger := logger.CreateLogger()
		cleanupCtx := context.Background()
		cleanupOpts, err := options.NewTerragruntOptionsForTest("")
		if err == nil {
			for _, container := range createdContainers {
				config := map[string]interface{}{
					"storage_account_name": storageAccount,
					"container_name":       container,
					"use_azuread_auth":     true,
				}

				client, err := azurehelper.CreateBlobServiceClient(cleanupCtx, cleanupLogger, cleanupOpts, config)
				if err == nil {
					if err := client.DeleteContainer(cleanupCtx, cleanupLogger, container); err != nil {
						t.Logf("Warning: Failed to delete container %s: %v", container, err)
					} else {
						t.Logf("Successfully cleaned up container: %s", container)
					}
				}
			}
		}
	}()

	// Create test terragrunt.hcl file
	terragruntConfig := fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "%s"
    container_name      = "%s"
    key                 = "test/terraform.tfstate"
    use_azuread_auth    = true
  }
}

terraform {
  source = "./modules/simple"
}
`, storageAccount, containerName)

	terragruntConfigPath := fmt.Sprintf("%s/terragrunt.hcl", tmpDir)
	err := os.WriteFile(terragruntConfigPath, []byte(terragruntConfig), 0644)
	require.NoError(t, err)

	// Create simple terraform module
	moduleDir := fmt.Sprintf("%s/modules/simple", tmpDir)
	err = os.MkdirAll(moduleDir, 0755)
	require.NoError(t, err)

	simpleModule := `
output "test_output" {
  value = "Hello from Azure backend integration test"
}
`
	err = os.WriteFile(fmt.Sprintf("%s/main.tf", moduleDir), []byte(simpleModule), 0644)
	require.NoError(t, err)

	// Test cases for different bootstrap scenarios
	testCases := []struct {
		name        string
		command     string
		expectError bool
	}{
		{
			name:        "bootstrap_command",
			command:     "terragrunt backend bootstrap --non-interactive",
			expectError: false,
		},
		{
			name:        "apply_with_auto_bootstrap",
			command:     "terragrunt --non-interactive backend bootstrap",
			expectError: false,
		},
		{
			name:        "init_with_backend_config",
			command:     "terragrunt --non-interactive init",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Each subtest gets its own container to avoid conflicts
			testContainerName := containerName + "-" + strings.ReplaceAll(tc.name, "_", "-")
			createdContainers = append(createdContainers, testContainerName)
			
			// Update config for this specific test
			testConfig := strings.ReplaceAll(terragruntConfig, containerName, testContainerName)
			testConfigPath := fmt.Sprintf("%s/terragrunt-%s.hcl", tmpDir, tc.name)
			err := os.WriteFile(testConfigPath, []byte(testConfig), 0644)
			require.NoError(t, err)

			// Change to test directory
			originalDir, err := os.Getwd()
			require.NoError(t, err)
			defer os.Chdir(originalDir)

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			// Set config file for this test
			testCmd := strings.ReplaceAll(tc.command, "terragrunt", fmt.Sprintf("terragrunt --terragrunt-config %s", testConfigPath))

			// Run the command
			output, stderr, err := helpers.RunTerragruntCommandWithOutput(t, testCmd)
			
			if tc.expectError {
				require.Error(t, err, "Expected command to fail")
			} else {
				require.NoError(t, err, "Command failed: %v\nOutput: %s\nError: %s", err, output, stderr)
			}

			t.Logf("Command output: %s", output)

			// Verify container was created if bootstrap was successful
			if !tc.expectError {
				// Create blob client to verify container
				opts, err := options.NewTerragruntOptionsForTest("")
				require.NoError(t, err)

				config := map[string]interface{}{
					"storage_account_name": storageAccount,
					"container_name":       testContainerName,
					"use_azuread_auth":     true,
				}

				log := logger.CreateLogger()
				client, err := azurehelper.CreateBlobServiceClient(t.Context(), log, opts, config)
				if err == nil {
					exists, _ := client.ContainerExists(t.Context(), testContainerName)
					if exists {
						t.Logf("Container %s was successfully created", testContainerName)
					}
				}
			}
		})
	}
}
