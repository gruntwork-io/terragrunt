package azure

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/stretchr/testify/assert"
)

func TestConfigMissingStorageAccountName(t *testing.T) {
	t.Parallel()

	conf := map[string]string{}

	_, err := New(conf)
	assert.NotNil(t, err)
	assert.True(t, errors.IsError(err, ErrRequiredFieldMissing("storage_account_name")))
}

func TestConfigMissingContainerName(t *testing.T) {
	t.Parallel()

	conf := map[string]string{
		"storage_account_name": "account",
	}

	_, err := New(conf)
	assert.NotNil(t, err)
	assert.True(t, errors.IsError(err, ErrRequiredFieldMissing("container_name")))
}

func TestConfigMissingKey(t *testing.T) {
	t.Parallel()

	conf := map[string]string{
		"storage_account_name": "account",
		"container_name":       "container",
	}

	_, err := New(conf)
	assert.NotNil(t, err)
	assert.True(t, errors.IsError(err, ErrRequiredFieldMissing("key")))
}

func TestConfigValid(t *testing.T) {
	t.Parallel()

	conf := map[string]string{
		"storage_account_name": "account",
		"container_name":       "container",
		"key":                  "key",
	}

	lock, err := New(conf)
	assert.NotNil(t, lock)
	assert.IsType(t, &StorageLock{}, lock)
	assert.Nil(t, err)

	storageLock := lock.(*StorageLock)
	assert.Equal(t, "account", storageLock.StorageAccountName)
	assert.Equal(t, "container", storageLock.ContainerName)
	assert.Equal(t, "key", storageLock.Key)
}

func TestAcquireLockContainerNotFoundError(t *testing.T) {
	t.Parallel()

	err := setupAzureAccTest(func(storageAccount, container string, client storage.BlobStorageClient) {
		lock := StorageLock{
			StorageAccountName: storageAccount,
			ContainerName:      "bad-container-name",
			Key:                "TestAcquireLockContainerNotFoundError.lock",
		}

		err := lock.AcquireLock()
		assert.NotNil(t, err)
		assert.Regexp(t, "The specified container does not exist", err.Error())
	})

	assert.Nil(t, err)
}

func TestAcquireAndReleaseLock(t *testing.T) {
	t.Parallel()

	err := setupAzureAccTest(func(storageAccount, container string, client storage.BlobStorageClient) {
		lock := StorageLock{
			StorageAccountName: storageAccount,
			ContainerName:      container,
			Key:                "TestAcquireLock.lock",
		}

		// acquire, confirm lock file was created
		err := lock.AcquireLock()
		assert.Nil(t, err)
		exists, err := client.BlobExists(container, lock.Key)
		assert.Nil(t, err)
		assert.True(t, exists)

		// release, confirm lock file was deleted
		err = lock.ReleaseLock()
		assert.Nil(t, err)
		exists, err = client.BlobExists(container, lock.Key)
		assert.Nil(t, err)
		assert.False(t, exists)
	})

	assert.Nil(t, err)
}

func TestAcquireLockAlreadyLockedError(t *testing.T) {
	t.Parallel()

	err := setupAzureAccTest(func(storageAccount, container string, client storage.BlobStorageClient) {
		lock := StorageLock{
			StorageAccountName: storageAccount,
			ContainerName:      container,
			Key:                "TestAcquireLockAlreadyLockedError.lock",
		}

		err := lock.AcquireLock()
		assert.Nil(t, err)

		err = lock.AcquireLock()
		assert.NotNil(t, err)
		assert.True(t, err == ErrCannotLock)

		// cleanup
		err = lock.ReleaseLock()
		assert.Nil(t, err)
	})

	assert.Nil(t, err)
}

func setupAzureAccTest(testFunc func(storageAccount, container string, client storage.BlobStorageClient)) error {
	storageAccount := os.Getenv("AZURE_STORAGE_ACCOUNT")
	if storageAccount == "" {
		return fmt.Errorf("AZURE_STORAGE_ACCOUNT must be set for Azure lock tests")
	}

	container := os.Getenv("AZURE_STORAGE_CONTAINER")
	if container == "" {
		return fmt.Errorf("AZURE_STORAGE_CONTAINER must be set for Azure lock tests")
	}

	// uses ARM_ prefix to match Terraform
	accessKey := os.Getenv("ARM_ACCESS_KEY")
	if accessKey == "" {
		return fmt.Errorf("ARM_ACCESS_KEY must be set for Azure lock tests")
	}

	client, err := storage.NewBasicClient(storageAccount, accessKey)
	if err != nil {
		return err
	}

	testFunc(storageAccount, container, client.GetBlobService())
	return nil
}
