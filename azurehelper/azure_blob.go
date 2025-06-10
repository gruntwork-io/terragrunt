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
// NOTE: Storage account key authentication is deprecated and no longer supported. Please use Azure AD authentication instead.
func CreateBlobServiceClient(l log.Logger, opts *options.TerragruntOptions, config map[string]interface{}) (*BlobServiceClient, error) {
	// Get storage account name from config
	storageAccountName, ok := config["storage_account_name"].(string)
	if !ok || storageAccountName == "" {
		return nil, fmt.Errorf("storage_account_name is required")
	}

	url := fmt.Sprintf("https://%s.blob.core.windows.net", storageAccountName)

	// Use default Azure credential
	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting default azure credential: %v", err)
	}

	client, err := azblob.NewClient(url, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating blob client with default credential: %v", err)
	}

	return &BlobServiceClient{client: client}, nil
}

// GetObject downloads a blob from Azure Storage
func (c *BlobServiceClient) GetObject(input *GetObjectInput) (*GetObjectOutput, error) {
	if input.Bucket == nil || *input.Bucket == "" {
		return nil, fmt.Errorf("container name is required")
	}
	if input.Key == nil || *input.Key == "" {
		return nil, fmt.Errorf("blob key is required")
	}

	downloaded, err := c.client.DownloadStream(context.Background(), *input.Bucket, *input.Key, nil)
	if err != nil {
		if respErr, ok := err.(*azcore.ResponseError); ok && respErr.ErrorCode == "BlobNotFound" {
			return nil, fmt.Errorf("blob not found: %v", err)
		}
		return nil, fmt.Errorf("error downloading blob: %v", err)
	}

	return &GetObjectOutput{
		Body: downloaded.Body,
	}, nil
}

// ContainerExists checks if a container exists
func (c *BlobServiceClient) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if containerName == "" {
		return false, fmt.Errorf("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	_, err := container.GetProperties(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			// Container not found (404)
			if respErr.ErrorCode == "ContainerNotFound" {
				return false, nil
			}
			// Authentication error (401, 403)
			if respErr.StatusCode == 401 || respErr.StatusCode == 403 {
				return false, fmt.Errorf("authentication failed: %v", err)
			}
		}
		return false, fmt.Errorf("error checking container existence: %v", err)
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
			return fmt.Errorf("error creating container: %v", err)
		}
	}

	return nil
}

// IsVersioningEnabled checks if versioning is enabled for a blob storage account.
// In Azure Blob Storage, versioning is a storage account level setting and cannot be
// configured at the container level. This method always returns true as versioning
// state needs to be checked at the storage account level.
func (c *BlobServiceClient) IsVersioningEnabled(ctx context.Context, containerName string) (bool, error) {
	// In Azure Blob Storage, versioning is a storage account level setting
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
		if respErr, ok := err.(*azcore.ResponseError); ok && respErr.ErrorCode == "BlobNotFound" {
			return nil
		}
		return fmt.Errorf("error deleting blob: %v", err)
	}
	return nil
}

// DeleteContainer deletes a container and all its contents
func (c *BlobServiceClient) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	if containerName == "" {
		return fmt.Errorf("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	_, err := container.Delete(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			// If container not found, consider delete successful
			if respErr.ErrorCode == "ContainerNotFound" {
				return nil
			}
		}
		return fmt.Errorf("error deleting container: %w", err)
	}

	return nil
}

// UploadBlob uploads a blob with the given data
func (c *BlobServiceClient) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	if containerName == "" || blobName == "" {
		return fmt.Errorf("container name and blob key are required")
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
		return fmt.Errorf("container names and blob keys are required")
	}

	// Get source blob data
	input := &GetObjectInput{
		Bucket: &srcContainer,
		Key:    &srcKey,
	}
	srcBlob, err := c.GetObject(input)
	if err != nil {
		return fmt.Errorf("error getting source blob: %w", err)
	}
	defer srcBlob.Body.Close()

	// Read the entire blob into memory
	data, err := io.ReadAll(srcBlob.Body)
	if err != nil {
		return fmt.Errorf("error reading source blob: %w", err)
	}

	// Upload to destination using UploadBuffer which doesn't require a ReadSeekCloser
	container := dstClient.client.ServiceClient().NewContainerClient(dstContainer)
	blockBlob := container.NewBlockBlobClient(dstKey)

	_, err = blockBlob.UploadBuffer(ctx, data, nil)
	if err != nil {
		return fmt.Errorf("error uploading to destination: %w", err)
	}

	return nil
}

// DeleteBlob deletes a blob from a container
func (c *BlobServiceClient) DeleteBlob(ctx context.Context, containerName, blobKey string) error {
	if containerName == "" || blobKey == "" {
		return fmt.Errorf("container name and blob key are required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)
	blob := container.NewBlobClient(blobKey)

	_, err := blob.Delete(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			// If blob not found, consider delete successful
			if respErr.ErrorCode == "BlobNotFound" {
				return nil
			}
		}
		return fmt.Errorf("error deleting blob: %w", err)
	}

	return nil
}
