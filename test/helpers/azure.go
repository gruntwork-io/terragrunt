// Package helpers provides test helper functions
package helpers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
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
	containerName := fmt.Sprintf("tg%s", strings.ReplaceAll(uniqueID, "_", "-"))
	if len(containerName) > 63 {
		containerName = containerName[:63]
	}
	if containerName == "" {
		t.Fatal("Generated container name is empty")
	}
	if len(containerName) < 3 {
		t.Fatal("Generated container name is too short - must be at least 3 characters")
	}
	location := os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION")

	if accountName == "" {
		t.Skip("Skipping Azure tests: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	if location == "" {
		location = "europewest" // Default location if not specified
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

	ctx := context.Background()
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
		require.NoError(t, err, fmt.Sprintf("Failed to delete container %s", config.ContainerName))
	}
}
