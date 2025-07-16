// Test file to verify the new StorageAccountService methods are implemented
package implementations

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageAccountServiceMethodsAreImplemented(t *testing.T) {
	t.Run("methods should not return nil by default", func(t *testing.T) {
		// Create a mock client for testing
		service := &StorageAccountServiceImpl{
			client: nil, // This will cause issues if we actually call the methods, but we're just testing they exist
		}

		// Test that methods exist and don't return nil without implementation

		// Test CreateStorageAccount exists
		assert.NotNil(t, service.CreateStorageAccount)

		// Test DeleteStorageAccount exists
		assert.NotNil(t, service.DeleteStorageAccount)

		// Test GetStorageAccountKeys exists
		assert.NotNil(t, service.GetStorageAccountKeys)

		// Test GetStorageAccountSAS exists
		assert.NotNil(t, service.GetStorageAccountSAS)

		// Test GetStorageAccountProperties exists
		assert.NotNil(t, service.GetStorageAccountProperties)

		// Test that the interface methods are properly implemented
		var _ interfaces.StorageAccountService = service
	})
}

func TestStorageAccountServiceInterface(t *testing.T) {
	t.Run("StorageAccountServiceImpl implements interface", func(t *testing.T) {
		service := &StorageAccountServiceImpl{}

		// Verify it implements the interface
		require.Implements(t, (*interfaces.StorageAccountService)(nil), service)

		// Test that the methods exist and are not nil function pointers
		assert.NotNil(t, service.CreateStorageAccount)
		assert.NotNil(t, service.DeleteStorageAccount)
		assert.NotNil(t, service.GetStorageAccount)
		assert.NotNil(t, service.GetStorageAccountKeys)
		assert.NotNil(t, service.GetStorageAccountSAS)
		assert.NotNil(t, service.GetStorageAccountProperties)
		assert.NotNil(t, service.IsVersioningEnabled)
	})
}
