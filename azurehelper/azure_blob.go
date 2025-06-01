// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// BlobServiceClient wraps Azure's azblob.Client to provide a simpler interface for our needs
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
	Body       ResponseBodyCloser
	Properties *azblob.BlobProperties
}

type ResponseBodyCloser interface {
	Read(p []byte) (n int, err error)
	Close() error
}

// CreateBlobServiceClient creates a new Azure Blob Service client using the configuration from the backend
func CreateBlobServiceClient(l log.Logger, opts *options.TerragruntOptions, config map[string]interface{}) (*BlobServiceClient, error) {
	storageAccountName, ok := config["storage_account_name"].(string)
	if !ok {
		return nil, fmt.Errorf("storage_account_name not found in backend config")
	}

	// Try different authentication methods in order:
	// 1. Connection string from backend config
	// 2. SAS Token from backend config
	// 3. Azure AD credentials from environment

	if connStr, ok := config["storage_account_key"].(string); ok {
		client, err := azblob.NewClientFromConnectionString(connStr, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating blob client from connection string: %v", err)
		}
		return &BlobServiceClient{client: client}, nil
	}

	if sasToken, ok := config["sas_token"].(string); ok {
		url := fmt.Sprintf("https://%s.blob.core.windows.net/?%s", storageAccountName, sasToken)
		client, err := azblob.NewClientWithNoCredential(url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating blob client with SAS token: %v", err)
		}
		return &BlobServiceClient{client: client}, nil
	}

	// Default to Azure AD authentication
	cred, err := azblob.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("error getting default Azure credentials: %v", err)
	}

	url := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)
	client, err := azblob.NewClient(url, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating blob client: %v", err)
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

	// Download the blob
	resp, err := c.client.DownloadStream(context.Background(), *input.Bucket, *input.Key, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if os.IsNotExist(err) || (storageErr != nil && storageErr.ErrorCode == "BlobNotFound") {
			return nil, fmt.Errorf("blob not found: %v", err)
		}
		return nil, fmt.Errorf("error downloading blob: %v", err)
	}

	return &GetObjectOutput{
		Body:       resp.Body,
		Properties: resp.BlobProperties,
	}, nil
}

// ContainerExists checks if a container exists
func (c *BlobServiceClient) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	containerClient := c.client.NewContainerClient(containerName)
	_, err := containerClient.GetProperties(ctx, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if storageErr != nil && storageErr.ErrorCode == "ContainerNotFound" {
			return false, nil
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
		containerClient := c.client.NewContainerClient(containerName)
		_, err = containerClient.Create(ctx, nil)
		if err != nil {
			return fmt.Errorf("error creating container: %v", err)
		}
	}

	return nil
}

// EnableVersioningIfNecessary enables versioning on a container if not already enabled
func (c *BlobServiceClient) EnableVersioningIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	enabled, err := c.IsVersioningEnabled(ctx, containerName)
	if err != nil {
		return err
	}

	if !enabled {
		l.Infof("Enabling versioning on Azure Storage container %s", containerName)
		containerClient := c.client.NewContainerClient(containerName)
		_, err = containerClient.SetVersioningEnabled(ctx, true, nil)
		if err != nil {
			return fmt.Errorf("error enabling versioning: %v", err)
		}
	}

	return nil
}

// IsVersioningEnabled checks if versioning is enabled on a container
func (c *BlobServiceClient) IsVersioningEnabled(ctx context.Context, containerName string) (bool, error) {
	containerClient := c.client.NewContainerClient(containerName)
	props, err := containerClient.GetProperties(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("error getting container properties: %v", err)
	}

	return props.VersioningEnabled != nil && *props.VersioningEnabled, nil
}

// DeleteBlobIfNecessary deletes a blob if it exists
func (c *BlobServiceClient) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	containerClient := c.client.NewContainerClient(containerName)
	blobClient := containerClient.BlockBlobClient(blobName)
	_, err := blobClient.Delete(ctx, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if storageErr != nil && storageErr.ErrorCode == "BlobNotFound" {
			return nil
		}
		return fmt.Errorf("error deleting blob: %v", err)
	}
	return nil
}

// DeleteContainer deletes a container and all its contents
func (c *BlobServiceClient) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	containerClient := c.client.NewContainerClient(containerName)
	_, err := containerClient.Delete(ctx, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if storageErr != nil && storageErr.ErrorCode == "ContainerNotFound" {
			return nil
		}
		return fmt.Errorf("error deleting container: %v", err)
	}
	return nil
}
