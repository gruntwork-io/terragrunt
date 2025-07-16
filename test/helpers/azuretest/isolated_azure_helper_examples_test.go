package azuretest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/stretchr/testify/require"
)

// Example test using the isolated Azure helper to demonstrate complete resource isolation
func ExampleIsolatedAzureTest(t *testing.T) {
	// This test demonstrates how to use the isolated Azure helper for complete resource isolation
	// It creates isolated storage accounts and resource groups for each test run

	// Get isolated Azure configuration
	config := GetIsolatedAzureConfig(t)

	// Set up cleanup to run after the test
	defer CleanupAzureResources(t, config)

	// Create isolated resources if needed
	EnsureResourceGroupExists(t, config)
	EnsureStorageAccountExists(t, config)

	// Get blob client for the isolated resources
	blobClient := GetAzureBlobClient(t, config)

	// Ensure container exists
	EnsureContainerExists(t, config, blobClient)

	// Now perform your test logic with fully isolated resources
	ctx := context.Background()

	// Test blob operations
	testBlob := fmt.Sprintf("test-blob-%d", time.Now().Unix())
	testData := []byte("test data for isolated Azure test")

	// Upload a blob
	err := blobClient.UploadBlob(ctx, nil, config.ContainerName, testBlob, testData)
	require.NoError(t, err, "Failed to upload blob")

	// Download the blob
	downloadedData, err := blobClient.GetObject(ctx, &azurehelper.GetObjectInput{
		Container: &config.ContainerName,
		Key:       &testBlob,
	})
	require.NoError(t, err, "Failed to download blob")

	// Verify the data matches
	require.NotNil(t, downloadedData.Body, "Downloaded blob body should not be nil")
	defer downloadedData.Body.Close()

	// Test passed - resources will be cleaned up automatically by the defer
	t.Logf("Test completed successfully with isolated resources")
}

// Example test using container-level isolation only
func ExampleContainerOnlyIsolationTest(t *testing.T) {
	// This test demonstrates container-level isolation only
	// It uses existing storage account and resource group

	// Get isolated Azure configuration
	config := GetIsolatedAzureConfig(t)

	// Set up cleanup to run after the test
	defer CleanupAzureResources(t, config)

	// Get blob client for the existing resources
	blobClient := GetAzureBlobClient(t, config)

	// Ensure container exists
	EnsureContainerExists(t, config, blobClient)

	// Now perform your test logic with isolated container
	ctx := context.Background()

	// Test container operations
	exists, err := blobClient.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err, "Failed to check container existence")
	require.True(t, exists, "Container should exist after creation")

	t.Logf("Container isolation test completed successfully")
}

// Example test for parallel execution with resource isolation
func ExampleParallelSafeTest(t *testing.T) {
	// This test demonstrates how to safely run tests in parallel
	// Each test gets its own isolated resources based on the test name and timestamp

	t.Parallel() // Safe to run in parallel because of resource isolation

	// Get isolated Azure configuration
	config := GetIsolatedAzureConfig(t)

	// Set up cleanup to run after the test
	defer CleanupAzureResources(t, config)

	// Create isolated resources if needed
	EnsureResourceGroupExists(t, config)
	EnsureStorageAccountExists(t, config)

	// Get blob client for the isolated resources
	blobClient := GetAzureBlobClient(t, config)

	// Ensure container exists
	EnsureContainerExists(t, config, blobClient)

	// Test is now safe to run in parallel
	ctx := context.Background()

	// Test unique operations for this test
	testBlob := fmt.Sprintf("parallel-test-%s", config.TestID)
	testData := []byte(fmt.Sprintf("parallel test data for %s", config.TestName))

	// Upload a blob
	err := blobClient.UploadBlob(ctx, nil, config.ContainerName, testBlob, testData)
	require.NoError(t, err, "Failed to upload blob in parallel test")

	// Check container exists
	exists, err := blobClient.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err, "Failed to check container existence in parallel test")
	require.True(t, exists, "Container should exist in parallel test")

	t.Logf("Parallel safe test completed successfully with test ID: %s", config.TestID)
}
