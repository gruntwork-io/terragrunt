// Package testing provides test utilities for Azure remote state backends
package testing

import (
	"os"
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
		t.Logf("Using Azure AD authentication for tests (TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set)")
	} else {
		t.Logf("Using Storage Account Key authentication for tests")
	}

	return storageAccount, accessKey
}

// GetAzureTestConfig gets Azure storage test config from env vars
func GetAzureTestConfig(t *testing.T) *AzureTestConfig {
	t.Helper()

	accountName, accessKey := CheckAzureTestCredentials(t)

	// Generate Azure-compliant container name:
	// - 3-63 chars
	// - Only lowercase letters, numbers, hyphens
	// - Start/end with letter/number
	// - No consecutive hyphens
	uniqueID := strings.ToLower(t.Name())
	uniqueID = strings.ReplaceAll(uniqueID, "_", "-")
	uniqueID = strings.ReplaceAll(uniqueID, "/", "-")
	containerName := "tg-test-" + uniqueID

	if len(containerName) > AzureStorageContainerMaxLength {
		containerName = containerName[:AzureStorageContainerMaxLength]
	}

	if len(containerName) < AzureStorageContainerMinLength {
		containerName = "tg-" + uniqueID
		if len(containerName) > AzureStorageContainerMaxLength {
			containerName = containerName[:AzureStorageContainerMaxLength]
		}

		if len(containerName) < AzureStorageContainerMinLength {
			containerName = "tgz"
		}
	}

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
