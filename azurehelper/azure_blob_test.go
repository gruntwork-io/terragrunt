package azurehelper

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogger(t *testing.T) log.Logger {
	t.Helper()
	return logger.CreateLogger()
}

func TestCreateBlobServiceClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing-storage-account",
			config: map[string]interface{}{
				"container_name": "test-container",
			},
			expectError: true,
			errorMsg:    "storage_account_name is required",
		},
		{
			name: "with-default-credentials",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable for parallel testing
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			logger := createTestLogger(t)
			client, err := CreateBlobServiceClient(logger, opts, tc.config)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, client)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
			}
		})
	}
}

func TestBlobOperations(t *testing.T) {
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" {
		t.Skip("Skipping Azure blob operations test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	t.Parallel()

	ctx := context.Background()
	containerName := fmt.Sprintf("test-container-%d", os.Getpid())
	blobName := "test-blob.txt"

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	config := map[string]interface{}{
		"storage_account_name": os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
		"container_name":       containerName,
	}

	logger := createTestLogger(t)
	client, err := CreateBlobServiceClient(logger, opts, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test container creation
	err = client.CreateContainerIfNecessary(ctx, logger, containerName)
	require.NoError(t, err)

	// Test container existence check
	exists, err := client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists)

	enabled, err := client.IsVersioningEnabled(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, enabled)

	// Test blob operations
	input := &GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	}

	// Test get non-existent blob
	_, err = client.GetObject(input)
	assert.Error(t, err)

	// Test delete non-existent blob
	err = client.DeleteBlobIfNecessary(ctx, logger, containerName, blobName)
	assert.NoError(t, err)

	// Clean up
	err = client.DeleteContainer(ctx, logger, containerName)
	require.NoError(t, err)

	// Verify container deletion
	exists, err = client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestContainerOperationsWithErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	invalidContainerName := "invalid$container"

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Use an invalid/non-existent storage account
	config := map[string]interface{}{
		"storage_account_name": "nonexistentaccount",
	}

	logger := createTestLogger(t)
	client, err := CreateBlobServiceClient(logger, opts, config)
	require.NoError(t, err)

	// Test container creation with invalid name
	err = client.CreateContainerIfNecessary(ctx, logger, invalidContainerName)
	assert.Error(t, err)

	// Test container existence check with invalid name
	exists, err := client.ContainerExists(ctx, invalidContainerName)
	assert.Error(t, err)
	assert.False(t, exists)
}

func TestContainerExists(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		config         map[string]interface{}
		containerName  string
		expectError    bool
		expectedExists bool
	}{
		{
			name: "empty-container-name",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
			},
			containerName:  "",
			expectError:    true,
			expectedExists: false,
		},
		{
			name: "non-existent-container",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
			},
			containerName:  "non-existent-container",
			expectError:    true, // Will fail with auth error since we don't have valid credentials
			expectedExists: false,
		},
		{
			name: "invalid-storage-account",
			config: map[string]interface{}{
				"storage_account_name": "nonexistentaccount",
			},
			containerName:  "test-container",
			expectError:    true,
			expectedExists: false,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable for parallel testing
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			opts, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			logger := createTestLogger(t)
			client, err := CreateBlobServiceClient(logger, opts, tc.config)
			require.NoError(t, err)
			require.NotNil(t, client)

			exists, err := client.ContainerExists(ctx, tc.containerName)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedExists, exists)
			}
		})
	}
}

func TestContainerExistsIntegration(t *testing.T) {
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" {
		t.Skip("Skipping Azure container existence test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	ctx := context.Background()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	config := map[string]interface{}{
		"storage_account_name": os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
	}

	logger := createTestLogger(t)
	client, err := CreateBlobServiceClient(logger, opts, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	testContainerName := fmt.Sprintf("terragrunt-test-%d", time.Now().UnixNano())

	// Test non-existent container first
	exists, err := client.ContainerExists(ctx, testContainerName)
	assert.NoError(t, err)
	assert.False(t, exists)

	// Create the container and test again
	err = client.CreateContainerIfNecessary(ctx, logger, testContainerName)
	require.NoError(t, err)

	// Ensure cleanup even if subsequent tests fail
	defer func() {
		err := client.DeleteContainer(ctx, logger, testContainerName)
		if err != nil {
			t.Logf("Warning: Failed to cleanup container %s: %v", testContainerName, err)
		}
	}()

	// Verify container exists after creation
	exists, err = client.ContainerExists(ctx, testContainerName)
	assert.NoError(t, err)
	assert.True(t, exists)
}
