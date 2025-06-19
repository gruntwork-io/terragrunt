// Package helpers provides test helper functions
package helpers

import (
	"os"
	"strings"
	"testing"

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

// AzureStorageTestConfig contains Azure storage test configuration
type AzureStorageTestConfig struct {
	StorageAccountName string
	ContainerName      string
	Location           string
}

// GetAzureStorageTestConfig gets Azure storage test config from env vars
func GetAzureStorageTestConfig(t *testing.T) *AzureStorageTestConfig {
	t.Helper()

	accountName := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")

	// Generate Azure-compliant container name:
	// - 3-63 chars
	// - Only lowercase letters, numbers, hyphens
	// - Start/end with letter/number
	// - No consecutive hyphens
	uniqueID := strings.ToLower(UniqueID())
	containerName := "tg" + strings.ReplaceAll(uniqueID, "_", "-")

	if len(containerName) > AzureStorageContainerMaxLength {
		containerName = containerName[:AzureStorageContainerMaxLength]
	}

	if containerName == "" {
		t.Fatal("Generated container name is empty")
	}

	if len(containerName) < AzureStorageContainerMinLength {
		t.Fatal("Generated container name is too short - must be at least 3 characters")
	}

	location := os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION")

	if accountName == "" {
		t.Skip("Skipping Azure tests: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	if location == "" {
		location = "westeurope" // Default location if not specified
	}

	return &AzureStorageTestConfig{
		StorageAccountName: accountName,
		ContainerName:      containerName,
		Location:           location,
	}
}

// CleanupAzureContainer deletes an Azure storage container
func CleanupAzureContainer(t *testing.T, config *AzureStorageTestConfig) {
	t.Helper()

	if config == nil {
		return
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()
	logger := log.Default()

	// Create storage client using Azure AD authentication
	client, err := azurehelper.CreateBlobServiceClient(logger, opts, map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"use_azuread_auth":     true,
	})
	require.NoError(t, err)

	// Delete container if exists
	exists, err := client.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err)

	if exists {
		err = client.DeleteContainer(ctx, logger, config.ContainerName)
		require.NoError(t, err, "Failed to delete container "+config.ContainerName)
	}
}
