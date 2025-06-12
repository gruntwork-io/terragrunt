// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// BlobServiceClient wraps Azure's azblob client to provide a simpler interface for our needs
type BlobServiceClient struct {
	client *azblob.Client
	config map[string]interface{}
}

// GetObjectInput represents input parameters for getting a blob
type GetObjectInput struct {
	Bucket *string
	Key    *string
}

// GetObjectOutput represents the output from getting a blob
type GetObjectOutput struct {
	Body io.ReadCloser
}

// CreateBlobServiceClient creates a new Azure Blob Service client using the configuration from the backend
func CreateBlobServiceClient(l log.Logger, opts *options.TerragruntOptions, config map[string]interface{}) (*BlobServiceClient, error) {
	storageAccountName, okStorageAccountName := config["storage_account_name"].(string)
	if !okStorageAccountName || storageAccountName == "" {
		return nil, errors.New("storage_account_name is required")
	}

	url := fmt.Sprintf("https://%s.blob.core.windows.net", storageAccountName)

	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting default azure credential: %w", err)
	}

	client, err := azblob.NewClient(url, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating blob client with default credential: %w", err)
	}

	return &BlobServiceClient{
		client: client,
		config: config,
	}, nil
}

// GetObject downloads a blob from Azure Storage
func (c *BlobServiceClient) GetObject(ctx context.Context, input *GetObjectInput) (*GetObjectOutput, error) {
	if input.Bucket == nil || *input.Bucket == "" {
		return nil, errors.New("container name is required")
	}

	if input.Key == nil || *input.Key == "" {
		return nil, errors.New("blob key is required")
	}

	downloaded, err := c.client.DownloadStream(ctx, *input.Bucket, *input.Key, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "BlobNotFound" {
			return nil, fmt.Errorf("blob not found: %w", err)
		}

		return nil, fmt.Errorf("error downloading blob: %w", err)
	}

	return &GetObjectOutput{
		Body: downloaded.Body,
	}, nil
}

// ContainerExists checks if a container exists
func (c *BlobServiceClient) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if containerName == "" {
		return false, errors.New("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	_, err := container.GetProperties(ctx, nil)

	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.ErrorCode == "ContainerNotFound" {
				return false, nil
			}

			if respErr.StatusCode == 401 || respErr.StatusCode == 403 {
				return false, fmt.Errorf("authentication failed: %w", err)
			}
		}

		return false, fmt.Errorf("error checking container existence: %w", err)
	}

	return true, nil
}

// CreateContainerIfNecessary creates a container if it doesn't exist
func (c *BlobServiceClient) CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	exists, err := c.ContainerExists(ctx, containerName)
	if err != nil {
		return err
	}

	if !exists {
		l.Infof("Creating Azure Storage container %s", containerName)
		_, err = c.client.CreateContainer(ctx, containerName, nil)

		if err != nil {
			return fmt.Errorf("error creating container: %w", err)
		}
	}

	return nil
}

// IsVersioningEnabled checks if versioning is enabled for a blob storage account.
// In Azure Blob Storage, versioning is a storage account level setting and cannot be
// configured at the container level. This method always returns true as versioning
// state needs to be checked at the storage account level.
func (c *BlobServiceClient) IsVersioningEnabled(ctx context.Context, containerName string) (bool, error) {
	l := log.Default()
	l.Warnf("Warning: Blob versioning in Azure Storage is a storage account level setting, not a container level setting")

	return true, nil
}

// EnableVersioningIfNecessary is deprecated as versioning is a storage account level setting
func (c *BlobServiceClient) EnableVersioningIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	l.Warnf("Warning: Blob versioning in Azure Storage is a storage account level setting and cannot be configured at container level")

	return nil
}

// DeleteBlobIfNecessary deletes a blob if it exists
func (c *BlobServiceClient) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	_, err := c.client.DeleteBlob(ctx, containerName, blobName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "BlobNotFound" {
			return nil
		}

		return fmt.Errorf("error deleting blob: %w", err)
	}

	return nil
}

// DeleteContainer deletes a container and all its contents
func (c *BlobServiceClient) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	if containerName == "" {
		return errors.New("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	_, err := container.Delete(ctx, nil)

	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "ContainerNotFound" {
			return nil
		}

		return fmt.Errorf("error deleting container: %w", err)
	}

	return nil
}

// UploadBlob uploads a blob with the given data
func (c *BlobServiceClient) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	if containerName == "" || blobName == "" {
		return errors.New("container name and blob key are required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	blockBlob := container.NewBlockBlobClient(blobName)

	_, err := blockBlob.UploadBuffer(ctx, data, nil)
	if err != nil {
		return fmt.Errorf("error uploading blob: %w", err)
	}

	return nil
}

// CopyBlobToContainer copies a blob from one container to another, potentially across storage accounts
func (c *BlobServiceClient) CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient *BlobServiceClient, dstContainer, dstKey string) error {
	if srcContainer == "" || srcKey == "" || dstContainer == "" || dstKey == "" {
		return errors.New("container names and blob keys are required")
	}

	input := &GetObjectInput{
		Bucket: &srcContainer,
		Key:    &srcKey,
	}

	srcBlob, err := c.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("error getting source blob: %w", err)
	}

	defer func() {
		if closeErr := srcBlob.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing blob body: %w", closeErr)
		}
	}()

	// Read the source blob data
	data, err := io.ReadAll(srcBlob.Body)
	if err != nil {
		return fmt.Errorf("error reading source blob: %w", err)
	}

	// Upload to destination
	err = dstClient.UploadBlob(ctx, log.Default(), dstContainer, dstKey, data)
	if err != nil {
		return fmt.Errorf("error uploading to destination: %w", err)
	}

	return nil
}
