// Package testing provides test utilities for Azure remote state backends
package testing

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

const (
	// AzureStorageContainerMinLength is the minimum length for Azure storage container names (3 characters)
	AzureStorageContainerMinLength = 3
	// AzureStorageContainerMaxLength is the maximum length for Azure storage container names (63 characters)
	AzureStorageContainerMaxLength = 63
)

// AzureTestConfig contains Azure storage test configuration
type AzureTestConfig struct {
	// Group string fields together
	StorageAccountName string
	ContainerName      string
	Location           string
	AccessKey          string
	// Put bool field at the end
	UseAzureAD bool
	// Add padding to optimize struct size
	_ [3]byte // padding to align to 8-byte boundary
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

// CleanupAzureContainer deletes an Azure storage container with retries
// nolint:mnd
func CleanupAzureContainer(t *testing.T, config *AzureTestConfig) {
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

	// Check if container exists
	exists, err := client.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err)

	if exists {
		// Delete container with retries
		maxRetries := 3

		var deleteErr error

		for i := 0; i < maxRetries; i++ {
			deleteErr = client.DeleteContainer(ctx, logger, config.ContainerName)
			if deleteErr == nil {
				break
			}

			time.Sleep(2 * time.Second)
		}

		require.NoError(t, deleteErr, "Failed to delete container "+config.ContainerName)

		// Verify container is deleted
		exists, err = client.ContainerExists(ctx, config.ContainerName)
		require.NoError(t, err)
		require.False(t, exists, "Container should be deleted")
	}
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

		time.Sleep(2 * time.Second)
	}

	require.NoError(t, checkErr)
	require.True(t, exists, "Container should exist")
}

// CleanupAzureContainerWithRetry deletes an Azure storage container with enhanced retry logic
func CleanupAzureContainerWithRetry(t *testing.T, config *AzureTestConfig, maxRetries int) {
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

	for i := 0; i < maxRetries; i++ {
		// Check if container exists first
		exists, err := client.ContainerExists(ctx, config.ContainerName)
		if err != nil {
			t.Logf("Error checking container existence (attempt %d): %v", i+1, err)
			continue
		}

		if !exists {
			t.Logf("Container %s already cleaned up", config.ContainerName)
			return // Already cleaned up
		}

		// Try to clean up
		err = client.DeleteContainer(ctx, logger, config.ContainerName)
		if err != nil {
			t.Logf("Cleanup attempt %d failed: %v", i+1, err)
		}

		// Wait a bit for Azure to process the deletion
		time.Sleep(2 * time.Second)

		// Verify deletion
		exists, err = client.ContainerExists(ctx, config.ContainerName)
		if err != nil {
			t.Logf("Error verifying container deletion (attempt %d): %v", i+1, err)
			time.Sleep(time.Duration(i+1) * 2 * time.Second) // Exponential backoff
			continue
		}

		if !exists {
			t.Logf("Successfully cleaned up container %s", config.ContainerName)
			return // Successfully cleaned up
		}

		if i == maxRetries-1 {
			t.Logf("Warning: Failed to cleanup container %s after %d attempts", config.ContainerName, maxRetries)
		} else {
			t.Logf("Cleanup attempt %d failed, retrying...", i+1)
			time.Sleep(time.Duration(i+1) * 2 * time.Second) // Exponential backoff
		}
	}
}

// generateUniqueContainerName creates a truly unique container name for Azure storage
// by combining test name with nanosecond timestamp to prevent parallel test conflicts
func generateUniqueContainerName(testName string) string {
	// Combine test name with nanosecond timestamp for true uniqueness
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	// Use last 8 digits of timestamp
	if len(timestamp) > 8 {
		timestamp = timestamp[len(timestamp)-8:]
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
