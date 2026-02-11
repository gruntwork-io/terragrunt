// Test file for BlobService implementation
//
//nolint:testpackage // Integration-style test exercises internal helpers.
package implementations

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlobServiceGetObjectValidation(t *testing.T) {
	t.Parallel()

	// Create a minimal service with nil client for validation testing
	service := &BlobServiceImpl{
		client: nil, // This will cause actual calls to fail, but validation should occur first
	}

	t.Run("nil input validation", func(t *testing.T) {
		t.Parallel()

		_, err := service.GetObject(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input cannot be nil")
	})

	t.Run("empty container validation", func(t *testing.T) {
		t.Parallel()

		input := &types.GetObjectInput{
			ContainerName: "",
			BlobName:      "test-blob",
		}
		_, err := service.GetObject(context.Background(), input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "container name is required")
	})

	t.Run("empty blob validation", func(t *testing.T) {
		t.Parallel()

		input := &types.GetObjectInput{
			ContainerName: "test-container",
			BlobName:      "",
		}
		_, err := service.GetObject(context.Background(), input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blob name is required")
	})
}

func TestBlobServiceInterface(t *testing.T) {
	t.Parallel()

	t.Run("BlobServiceImpl implements interface", func(t *testing.T) {
		t.Parallel()

		service := &BlobServiceImpl{}

		// Verify it implements the interface
		require.Implements(t, (*interfaces.BlobService)(nil), service)

		// Test that the methods exist and are not nil function pointers
		assert.NotNil(t, service.GetObject)
		assert.NotNil(t, service.ContainerExists)
		assert.NotNil(t, service.CreateContainerIfNecessary)
		assert.NotNil(t, service.DeleteContainer)
		assert.NotNil(t, service.DeleteBlobIfNecessary)
		assert.NotNil(t, service.UploadBlob)
		assert.NotNil(t, service.CopyBlobToContainer)
	})
}
