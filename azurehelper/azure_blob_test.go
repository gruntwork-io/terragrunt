package azurehelper

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBlobServiceClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
	}{
		{
			name: "missing-storage-account",
			config: map[string]interface{}{
				"container_name": "test-container",
			},
			expectError: true,
		},
		{
			name: "with-connection-string",
			config: map[string]interface{}{
				"storage_account_name":  "testaccount",
				"storage_account_key":   "DefaultEndpointsProtocol=https;AccountName=testaccount;AccountKey=testkey==;EndpointSuffix=core.windows.net",
				"container_name":        "test-container",
			},
			expectError: false,
		},
		{
			name: "with-sas-token",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"sas_token":           "sv=2020-08-04&ss=b&srt=sco&sp=rwdlacx&se=2021-08-07T05:16:17Z&st=2021-08-06T21:16:17Z&spr=https&sig=test",
				"container_name":       "test-container",
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

			client, err := CreateBlobServiceClient(log.Logger{}, opts, tc.config)
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestBlobOperations(t *testing.T) {
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" ||
		os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY") == "" {
		t.Skip("Skipping Azure blob operations test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT or TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set")
	}

	t.Parallel()

	ctx := context.Background()
	containerName := fmt.Sprintf("test-container-%d", os.Getpid())
	blobName := "test-blob.txt"

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	config := map[string]interface{}{
		"storage_account_name": os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
		"storage_account_key":  os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY"),
		"container_name":      containerName,
	}

	client, err := CreateBlobServiceClient(log.Logger{}, opts, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test container creation
	err = client.CreateContainerIfNecessary(ctx, log.Logger{}, containerName)
	require.NoError(t, err)

	// Test container existence check
	exists, err := client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists)

	// Test versioning
	err = client.EnableVersioningIfNecessary(ctx, log.Logger{}, containerName)
	require.NoError(t, err)

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
	err = client.DeleteBlobIfNecessary(ctx, log.Logger{}, containerName, blobName)
	assert.NoError(t, err)

	// Clean up
	err = client.DeleteContainer(ctx, log.Logger{}, containerName)
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

	config := map[string]interface{}{
		"storage_account_name": "testaccount",
		"storage_account_key":  "DefaultEndpointsProtocol=https;AccountName=testaccount;AccountKey=testkey==;EndpointSuffix=core.windows.net",
	}

	client, err := CreateBlobServiceClient(log.Logger{}, opts, config)
	require.NoError(t, err)

	// Test container creation with invalid name
	err = client.CreateContainerIfNecessary(ctx, log.Logger{}, invalidContainerName)
	assert.Error(t, err)

	// Test container existence check with invalid name
	exists, err := client.ContainerExists(ctx, invalidContainerName)
	assert.Error(t, err)
	assert.False(t, exists)

	// Test versioning with invalid name
	err = client.EnableVersioningIfNecessary(ctx, log.Logger{}, invalidContainerName)
	assert.Error(t, err)

	enabled, err := client.IsVersioningEnabled(ctx, invalidContainerName)
	assert.Error(t, err)
	assert.False(t, enabled)
}
