// Package testing provides test utilities for Azure remote state backends
package testing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

const (
	// AzureStorageContainerMinLength is the minimum length for Azure storage container names (3 characters)
	AzureStorageContainerMinLength = 3
	// AzureStorageContainerMaxLength is the maximum length for Azure storage container names (63 characters)
	AzureStorageContainerMaxLength = 63
	defaultCleanupSleepSeconds     = 2
	timestampSuffixLength          = 8
)

// AzureTestConfig contains Azure storage test configuration.
// This configuration is used during testing to configure Azure Storage backend
// operations in test environments. It supports both access key and Azure AD authentication.
//
// The configuration is typically populated from environment variables during test execution:
// - TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT: Storage account name
// - TERRAGRUNT_AZURE_TEST_ACCESS_KEY: Storage account access key (optional if using Azure AD)
// - TERRAGRUNT_AZURE_TEST_CONTAINER: Container name (optional, defaults to generated name)
// - TERRAGRUNT_AZURE_TEST_LOCATION: Azure region (optional, defaults to "eastus")
//
// Usage examples:
//
//	// Basic test configuration with access key
//	config := AzureTestConfig{
//	    StorageAccountName: "teststorageaccount",
//	    ContainerName:      "test-container",
//	    Location:           "eastus",
//	    AccessKey:          "access-key-value",
//	    UseAzureAD:         false,
//	}
//
//	// Test configuration with Azure AD authentication
//	config := AzureTestConfig{
//	    StorageAccountName: "teststorageaccount",
//	    ContainerName:      "test-container",
//	    Location:           "eastus",
//	    AccessKey:          "", // Empty when using Azure AD
//	    UseAzureAD:         true,
//	}
type AzureTestConfig struct {
	// StorageAccountName specifies the name of the Azure Storage account for testing.
	// Must be 3-24 characters long, contain only lowercase letters and numbers.
	// Should be a dedicated test storage account, not used for production.
	// Environment variable: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT
	StorageAccountName string

	// ContainerName specifies the name of the blob container for test operations.
	// Must be 3-63 characters long, contain only lowercase letters, numbers, and hyphens.
	// Test containers are typically cleaned up after test completion.
	// Environment variable: TERRAGRUNT_AZURE_TEST_CONTAINER
	ContainerName string

	// Location specifies the Azure region for test resources.
	// Must be a valid Azure region name (e.g., "eastus", "westus2").
	// Should be a region that supports all required Azure services.
	// Environment variable: TERRAGRUNT_AZURE_TEST_LOCATION
	// Default: "eastus"
	Location string

	// AccessKey specifies the storage account access key for authentication.
	// Required when UseAzureAD is false.
	// This is a sensitive value that should be handled securely in CI/CD environments.
	// Environment variable: TERRAGRUNT_AZURE_TEST_ACCESS_KEY
	// Optional when using Azure AD authentication.
	AccessKey string

	// UseAzureAD indicates whether to use Azure AD authentication instead of access keys.
	// When true, the test will use Azure AD authentication with automatic credential discovery.
	// When false, the test will use the provided AccessKey for authentication.
	// Azure AD authentication is preferred for security and credential management.
	// Default: false (use access key if provided)
	UseAzureAD bool

	// 3 bytes padding to align to 8-byte boundary for optimal memory layout
	_ [3]byte
}

// CheckAzureTestCredentials checks if the required Azure test credentials are available
// and skips the test if they are not. Returns the storage account name and access key if available.
func CheckAzureTestCredentials(t *testing.T) (storageAccount, accessKey string) {
	t.Helper() // Mark this as a test helper function

	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	accessKey = os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY")

	// Allow tests to run with Azure AD authentication if access key is not provided
	if storageAccount == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	// Log whether we're using Azure AD or access key authentication
	if accessKey == "" {
		t.Logf("[%s] Using Azure AD authentication for tests (TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set)", t.Name())
	} else {
		t.Logf("[%s] Using Storage Account Key authentication for tests", t.Name())
	}

	return storageAccount, accessKey
}

// GetAzureTestConfig gets Azure storage test config from env vars
func GetAzureTestConfig(t *testing.T) *AzureTestConfig {
	t.Helper()

	accountName, accessKey := CheckAzureTestCredentials(t)

	// Generate truly unique container name to prevent parallel test conflicts
	containerName := generateUniqueContainerName(t.Name())

	return &AzureTestConfig{
		StorageAccountName: accountName,
		ContainerName:      containerName,
		Location:           os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION"),
		UseAzureAD:         accessKey == "",
		AccessKey:          accessKey,
	}
}

// CleanupAzureContainer deletes an Azure storage container with advanced retry logic
// If maxRetries is not provided (set to 0), a default of 3 retries will be used
func CleanupAzureContainer(t *testing.T, config *AzureTestConfig, maxRetries ...int) {
	t.Helper()

	if config == nil {
		return
	}

	// Set default max retries if not specified
	retryLimit := 3
	if len(maxRetries) > 0 && maxRetries[0] > 0 {
		retryLimit = maxRetries[0]
	}

	client := createAzureBlobClient(t, config)

	for i := 0; i < retryLimit; i++ {
		if cleanupContainerAttempt(t, client, config.ContainerName, i, retryLimit) {
			return
		}
	}
}

// createAzureBlobClient creates a blob service client for testing
func createAzureBlobClient(t *testing.T, config *AzureTestConfig) *azurehelper.BlobServiceClient {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()
	logger := log.Default()

	// Create configuration based on authentication method
	azureConfig := map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
	}

	if config.UseAzureAD {
		azureConfig["use_azuread_auth"] = true
	} else {
		azureConfig["access_key"] = config.AccessKey
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, azureConfig)
	require.NoError(t, err)

	return client
}

// cleanupContainerAttempt attempts to clean up a container, returns true if successful
func cleanupContainerAttempt(t *testing.T, client *azurehelper.BlobServiceClient, containerName string, attempt, retryLimit int) bool {
	t.Helper()

	ctx := t.Context()
	logger := log.Default()

	// Check if container exists first
	exists, err := client.ContainerExists(ctx, containerName)
	if err != nil {
		t.Logf("Error checking container existence (attempt %d): %v", attempt+1, err)
		time.Sleep(time.Duration(attempt+1) * time.Second) // Exponential backoff

		return false
	}

	if !exists {
		t.Logf("Container %s already cleaned up or doesn't exist", containerName)

		return true // Already cleaned up
	}

	// Try to clean up
	if err = client.DeleteContainer(ctx, logger, containerName); err != nil {
		t.Logf("Cleanup attempt %d failed: %v", attempt+1, err)
	}

	// Wait a bit for Azure to process the deletion
	time.Sleep(defaultCleanupSleepSeconds * time.Second)

	// Verify deletion
	exists, err = client.ContainerExists(ctx, containerName)
	if err != nil {
		t.Logf("Error verifying container deletion (attempt %d): %v", attempt+1, err)
		time.Sleep(time.Duration(attempt+1) * defaultCleanupSleepSeconds * time.Second) // Exponential backoff

		return false
	}

	if !exists {
		t.Logf("Successfully cleaned up container %s", containerName)

		return true // Successfully cleaned up
	}

	if attempt == retryLimit-1 {
		// On the last attempt, we fail the test if cleanup doesn't succeed
		require.Fail(t, fmt.Sprintf("Failed to cleanup container %s after %d attempts", containerName, retryLimit))
	} else {
		t.Logf("Cleanup attempt %d failed, retrying...", attempt+1)
		time.Sleep(time.Duration(attempt+1) * defaultCleanupSleepSeconds * time.Second) // Exponential backoff
	}

	return false
}

// AssertContainerExists checks if an Azure storage container exists
// nolint:mnd
func AssertContainerExists(t *testing.T, config *AzureTestConfig) {
	t.Helper()

	if config == nil {
		return
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()
	logger := log.Default()

	// Create configuration based on authentication method
	azureConfig := map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
	}

	if config.UseAzureAD {
		azureConfig["use_azuread_auth"] = true
	} else {
		azureConfig["access_key"] = config.AccessKey
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, azureConfig)
	require.NoError(t, err)

	// Check if container exists with retries
	maxRetries := 3

	var exists bool

	var checkErr error

	for i := 0; i < maxRetries; i++ {
		exists, checkErr = client.ContainerExists(ctx, config.ContainerName)
		if checkErr == nil {
			break
		}

		time.Sleep(defaultCleanupSleepSeconds * time.Second)
	}

	require.NoError(t, checkErr)
	require.True(t, exists, "Container should exist")
}

// CleanupAzureContainerWithRetry is deprecated, use CleanupAzureContainer instead
// This function is kept for backward compatibility
func CleanupAzureContainerWithRetry(t *testing.T, config *AzureTestConfig, maxRetries int) {
	t.Helper()
	t.Logf("Warning: CleanupAzureContainerWithRetry is deprecated, use CleanupAzureContainer instead")
	CleanupAzureContainer(t, config, maxRetries)
}

// generateUniqueContainerName creates a truly unique container name for Azure storage
// by combining test name with nanosecond timestamp to prevent parallel test conflicts
func generateUniqueContainerName(testName string) string {
	// Combine test name with nanosecond timestamp for true uniqueness
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	// Use last digits of timestamp for uniqueness without exceeding limits
	if len(timestamp) > timestampSuffixLength {
		timestamp = timestamp[len(timestamp)-timestampSuffixLength:]
	}

	// Clean test name for Azure compliance
	cleanName := strings.ToLower(testName)
	cleanName = strings.ReplaceAll(cleanName, "/", "")
	cleanName = strings.ReplaceAll(cleanName, "_", "")
	cleanName = strings.ReplaceAll(cleanName, " ", "")
	cleanName = strings.ReplaceAll(cleanName, "test", "")

	// Ensure Azure container name compliance
	containerName := "tg-" + cleanName + "-" + timestamp
	if len(containerName) > AzureStorageContainerMaxLength {
		containerName = "tg-" + timestamp
	}

	return containerName
}
