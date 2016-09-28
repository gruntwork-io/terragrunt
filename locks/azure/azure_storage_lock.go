package azure

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/util"
)

// StorageLock provides a lock backed by Azure Storage
// Validate must be called prior to the lock being used
type StorageLock struct {
	StorageAccountName string
	ContainerName      string
	Key                string
}

// New is the factory function for StorageLock
func New(conf map[string]string) (locks.Lock, error) {
	lock := &StorageLock{
		StorageAccountName: conf["storage_account_name"],
		ContainerName:      conf["container_name"],
		Key:                conf["key"],
	}

	if lock.StorageAccountName == "" {
		return nil, errors.WithStackTrace(ErrRequiredFieldMissing("storage_account_name"))
	}

	if lock.ContainerName == "" {
		return nil, errors.WithStackTrace(ErrRequiredFieldMissing("container_name"))
	}

	if lock.Key == "" {
		return nil, errors.WithStackTrace(ErrRequiredFieldMissing("key"))
	}

	return lock, nil
}

// AcquireLock attempts to create a Blob in the Storage Container
func (lock *StorageLock) AcquireLock() error {
	util.Logger.Printf("Attempting to acquire lock for Blob key %s", lock.Key)

	client, err := lock.createStorageClient()
	if err != nil {
		return err
	}

	exists, err := client.BlobExists(lock.ContainerName, lock.Key)
	if err != nil {
		return err
	}
	if exists {
		return ErrCannotLock
	}

	if err = client.CreateBlockBlob(lock.ContainerName, lock.Key); err != nil {
		return err
	}

	util.Logger.Printf("Lock acquired!")
	return nil
}

// ReleaseLock attempts to delete the Blob in the Storage Container
func (lock *StorageLock) ReleaseLock() error {
	util.Logger.Printf("Attempting to release lock for Blob key %s", lock.Key)

	client, err := lock.createStorageClient()
	if err != nil {
		return err
	}

	if _, err = client.DeleteBlobIfExists(lock.ContainerName, lock.Key, nil); err != nil {
		return err
	}

	util.Logger.Printf("Lock released!")
	return nil
}

// String returns a description of this lock
func (lock *StorageLock) String() string {
	return fmt.Sprintf("AzureStorageLock lock for state file %s", lock.Key)
}

// createStorageClient creates a new Blob Storage Client from the Azure SDK
// returns and error if ARM_ACCESS_KEY is empty
func (lock *StorageLock) createStorageClient() (*storage.BlobStorageClient, error) {
	accessKey := os.Getenv("ARM_ACCESS_KEY")
	if accessKey == "" {
		return nil, errors.WithStackTrace(ErrRequiredFieldMissing("ARM_ACCESS_KEY environment variable"))
	}

	client, err := storage.NewBasicClient(lock.StorageAccountName, accessKey)
	if err != nil {
		return nil, err
	}

	blobClient := client.GetBlobService()
	return &blobClient, nil
}

var ErrCannotLock = fmt.Errorf("cannot acquire lock as it is currently held by another process")

type ErrRequiredFieldMissing string

func (err ErrRequiredFieldMissing) Error() string {
	return fmt.Sprintf("%s must be set", err)
}
