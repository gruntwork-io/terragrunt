// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// BlobServiceClient wraps Azure's azblob client to provide a simpler interface for our needs.
type BlobServiceClient struct {
	client *azblob.Client
	config map[string]interface{}
}

// GetObjectInput represents input parameters for getting a blob.
type GetObjectInput struct {
	Container *string
	Key       *string
}

// GetObjectOutput represents the output from getting a blob.
type GetObjectOutput struct {
	Body io.ReadCloser
}

// AzureResponseError represents an Azure API error response with detailed information.
// It contains the following fields:
//   - StatusCode: HTTP status code from the Azure API response
//   - ErrorCode: Azure-specific error code that identifies the error type
//   - Message: Human-readable error message describing what went wrong
type AzureResponseError struct {
	Message    string // Human-readable error message (larger field first)
	ErrorCode  string // Azure-specific error code
	StatusCode int    // HTTP status code from the Azure API response
}

// ConvertAzureError converts an azcore.ResponseError to AzureResponseError
func ConvertAzureError(err error) *AzureResponseError {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		// Extract the error message from the error object
		// since respErr.Message is not directly accessible
		message := respErr.Error()

		return &AzureResponseError{
			StatusCode: respErr.StatusCode,
			ErrorCode:  respErr.ErrorCode,
			Message:    message,
		}
	}

	return nil
}

// Error implements the error interface for AzureResponseError
func (e *AzureResponseError) Error() string {
	return fmt.Sprintf("Azure API error (StatusCode=%d, ErrorCode=%s): %s", e.StatusCode, e.ErrorCode, e.Message)
}

// CreateBlobServiceClient creates a new Azure Blob Service client using the configuration from the backend.
func CreateBlobServiceClient(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, config map[string]interface{}) (*BlobServiceClient, error) {
	storageAccountName, okStorageAccountName := config["storage_account_name"].(string)
	if !okStorageAccountName || storageAccountName == "" {
		return nil, errors.Errorf("storage_account_name is required")
	}

	// Extract resource group and subscription ID if provided
	resourceGroupName, _ := config["resource_group_name"].(string)
	subscriptionID, _ := config["subscription_id"].(string)

	var err error

	// If we have subscription ID and resource group, verify storage account exists using Management API
	if subscriptionID != "" && resourceGroupName != "" {
		// Create storage account client to verify the storage account exists
		saClient, err := CreateStorageAccountClient(ctx, l, config)
		if err != nil {
			return nil, errors.Errorf("error creating storage account client: %w", err)
		}

		// Check if the storage account exists
		exists, _, err := saClient.StorageAccountExists(ctx)
		if err != nil {
			return nil, errors.Errorf("error checking if storage account exists: %w", err)
		}

		if !exists {
			return nil, errors.Errorf("storage account %s does not exist in resource group %s",
				storageAccountName, resourceGroupName)
		}

		l.Infof("Verified storage account %s exists", storageAccountName)
	}

	// Support custom endpoints from config, default to public cloud
	endpointSuffix, ok := config["endpoint_suffix"].(string)
	if !ok || endpointSuffix == "" {
		// Try to get cloud environment and derive endpoint suffix
		if cloudEnv, ok := config["cloud_environment"].(string); ok && cloudEnv != "" {
			endpointSuffix = azureauth.GetEndpointSuffix(cloudEnv)
		} else {
			endpointSuffix = "core.windows.net" // Default to public cloud
		}
	}

	url := fmt.Sprintf("https://%s.blob.%s", storageAccountName, endpointSuffix)

	// Use the centralized auth package to get credentials
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, errors.Errorf("error getting azure auth config: %v", err)
	}

	authResult, err := azureauth.GetTokenCredential(ctx, l, authConfig)
	if err != nil {
		return nil, errors.Errorf("error getting azure credentials: %v", err)
	}

	var client *azblob.Client
	if authResult.Method == azureauth.AuthMethodSasToken {
		// For SAS token authentication, use a different client initialization
		client, err = azblob.NewClientWithNoCredential(url+"?"+authResult.SasToken, nil)
	} else {
		// For credential-based authentication methods
		client, err = azblob.NewClient(url, authResult.Credential, nil)
	}

	if err != nil {
		// Check if error is due to storage account not existing
		if strings.Contains(err.Error(), "not exist") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, errors.Errorf("storage account %s does not exist or is not accessible: %w",
				storageAccountName, err)
		}

		return nil, errors.Errorf("error creating blob client with default credential: %w", err)
	}
	// Check if we can access the service endpoint to verify the storage account exists and is accessible
	// Try to get properties of a non-existent container to test connectivity
	testContainerName := "terragrunt-connectivity-test"
	testContainer := client.ServiceClient().NewContainerClient(testContainerName)

	_, err = testContainer.GetProperties(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		switch {
		case errors.As(err, &respErr) && respErr.ErrorCode == "ContainerNotFound":
			// This is actually good - it means we reached the storage account but the container doesn't exist
			l.Infof("Successfully verified storage account %s exists and is accessible", storageAccountName)
		case errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound:
			// 404 can mean either the storage account doesn't exist or the container doesn't exist
			// Check the error code to differentiate
			if respErr.ErrorCode == "StorageAccountNotFound" || respErr.ErrorCode == "AccountNotFound" {
				return nil, errors.Errorf("storage account %s does not exist (HTTP %d: %s)",
					storageAccountName, respErr.StatusCode, respErr.ErrorCode)
			}
			// If it's just ContainerNotFound, that's actually expected and good
			l.Infof("Successfully verified storage account %s exists (container not found as expected)", storageAccountName)
		case errors.As(err, &respErr) && respErr.StatusCode == http.StatusForbidden:
			return nil, errors.Errorf("access denied to storage account %s: insufficient permissions (HTTP %d: %s). "+
				"Ensure you have 'Storage Blob Data Reader' or higher role assigned",
				storageAccountName, respErr.StatusCode, respErr.ErrorCode)
		case errors.As(err, &respErr) && respErr.StatusCode == http.StatusUnauthorized:
			return nil, errors.Errorf("authentication failed for storage account %s (HTTP %d: %s). "+
				"Check your Azure credentials and ensure they are valid",
				storageAccountName, respErr.StatusCode, respErr.ErrorCode)
		case errors.As(err, &respErr) && respErr.StatusCode >= 500:
			return nil, errors.Errorf("Azure service error when accessing storage account %s (HTTP %d: %s). "+
				"This may be a temporary issue, please try again",
				storageAccountName, respErr.StatusCode, respErr.ErrorCode)
		case errors.As(err, &respErr):
			// Other Azure response errors
			return nil, errors.Errorf("unexpected Azure API error when verifying storage account %s "+
				"(HTTP %d: %s): %w", storageAccountName, respErr.StatusCode, respErr.ErrorCode, err)
		default:
			// For non-Azure errors, check if it's a connectivity issue which suggests the storage account doesn't exist
			errMsg := err.Error()
			if strings.Contains(errMsg, "no such host") ||
				strings.Contains(errMsg, "dial tcp") ||
				strings.Contains(errMsg, "connection refused") ||
				strings.Contains(errMsg, "connection timeout") {
				return nil, errors.Errorf("storage account %s does not exist or is not accessible "+
					"(network error): %w", storageAccountName, err)
			}
			// For other errors, return a specific error message with context
			return nil, errors.Errorf("unexpected error verifying access to storage account %s: %w",
				storageAccountName, err)
		}
	}

	return &BlobServiceClient{
		client: client,
		config: config,
	}, nil
}

// GetObject downloads a blob from Azure Storage.
func (c *BlobServiceClient) GetObject(ctx context.Context, input *GetObjectInput) (*GetObjectOutput, error) {
	if input.Container == nil || *input.Container == "" {
		return nil, errors.Errorf("container name is required")
	}

	if input.Key == nil || *input.Key == "" {
		return nil, errors.Errorf("blob key is required")
	}

	downloaded, err := c.client.DownloadStream(ctx, *input.Container, *input.Key, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "BlobNotFound" {
			return nil, errors.Errorf("blob not found: %w", err)
		}

		return nil, errors.Errorf("error downloading blob: %w", err)
	}

	return &GetObjectOutput{
		Body: downloaded.Body,
	}, nil
}

// ContainerExists checks if a container exists.
func (c *BlobServiceClient) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if containerName == "" {
		return false, errors.Errorf("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)

	_, err := container.GetProperties(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.ErrorCode == "ContainerNotFound" {
				return false, nil
			}

			if respErr.StatusCode == http.StatusUnauthorized || respErr.StatusCode == http.StatusForbidden {
				return false, errors.Errorf("authentication failed: %w", err)
			}
		}

		return false, errors.Errorf("error checking container existence: %w", err)
	}

	return true, nil
}

// CreateContainerIfNecessary creates a container if it doesn't exist.
func (c *BlobServiceClient) CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	exists, err := c.ContainerExists(ctx, containerName)
	if err != nil {
		return err
	}

	if !exists {
		l.Infof("Creating Azure Storage container %s", containerName)

		_, err = c.client.CreateContainer(ctx, containerName, nil)
		if err != nil {
			return NewContainerCreationError(err, containerName)
		}
	}

	return nil
}

// DeleteBlobIfNecessary deletes a blob if it exists.
func (c *BlobServiceClient) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	if _, err := c.client.DeleteBlob(ctx, containerName, blobName, nil); err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "BlobNotFound" {
			return nil
		}

		return errors.Errorf("error deleting blob: %w", err)
	}

	return nil
}

// DeleteContainer deletes a container and all its contents.
func (c *BlobServiceClient) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	if containerName == "" {
		return errors.Errorf("container name is required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)

	_, err := container.Delete(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "ContainerNotFound" {
			return nil
		}

		return errors.Errorf("failed to delete Azure container %s: %w", containerName, err)
	}

	return nil
}

// UploadBlob uploads a blob with the given data.
func (c *BlobServiceClient) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	if containerName == "" || blobName == "" {
		return errors.Errorf("container name and blob key are required")
	}

	container := c.client.ServiceClient().NewContainerClient(containerName)

	blockBlob := container.NewBlockBlobClient(blobName)

	_, err := blockBlob.UploadBuffer(ctx, data, nil)
	if err != nil {
		return errors.Errorf("error uploading blob: %w", err)
	}

	return nil
}

// CopyBlobToContainer copies a blob from one container to another, potentially across storage accounts.
func (c *BlobServiceClient) CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient *BlobServiceClient,
	dstContainer, dstKey string,
) (err error) {
	if srcContainer == "" || srcKey == "" || dstContainer == "" || dstKey == "" {
		return errors.Errorf("container names and blob keys are required")
	}

	// Get source blob data
	input := &GetObjectInput{
		Container: &srcContainer,
		Key:       &srcKey,
	}

	srcBlobOutput, err := c.GetObject(ctx, input)
	if err != nil {
		return errors.Errorf("error reading source blob: %w", err)
	}

	// Using named return value with defer to handle errors properly
	defer func() {
		if closeErr := srcBlobOutput.Body.Close(); closeErr != nil && err == nil {
			// Only set the close error if we don't already have an error
			err = errors.Errorf("failed to close blob: %w", closeErr)
		}
	}()

	// Read the blob content
	blobData, err := io.ReadAll(srcBlobOutput.Body)
	if err != nil {
		return errors.Errorf("error reading blob data: %w", err)
	}

	// Create a logger for the upload operation to avoid nil pointer dereference
	logger := log.Default()

	// Upload to the destination
	if err := dstClient.UploadBlob(ctx, logger, dstContainer, dstKey, blobData); err != nil {
		return errors.Errorf("error copying blob to destination: %w", err)
	}

	return nil
}

// ContainerCreationError wraps errors that occur during Azure container operations.
type ContainerCreationError struct {
	Underlying    error  // 8 bytes (interface)
	ContainerName string // 16 bytes (string)
}

// NewContainerCreationError creates a new ContainerCreationError using the errors package for consistent error handling.
func NewContainerCreationError(underlying error, containerName string) ContainerCreationError {
	return ContainerCreationError{
		Underlying:    errors.New(underlying),
		ContainerName: containerName,
	}
}

// Error returns a string indicating that container operation failed.
func (err ContainerCreationError) Error() string {
	// Using the errors package for consistent formatting
	return fmt.Sprintf("error with container %s: %v", err.ContainerName, err.Underlying)
}

// Unwrap returns the underlying error that caused the container operation to fail.
func (err ContainerCreationError) Unwrap() error {
	return err.Underlying
}
