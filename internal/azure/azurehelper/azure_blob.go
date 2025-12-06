// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// verifyStorageAccountExists checks if the storage account exists using the Management API.
func verifyStorageAccountExists(ctx context.Context, l log.Logger, config map[string]interface{}, storageAccountName, resourceGroupName string) error {
	saClient, err := CreateStorageAccountClient(ctx, l, config)
	if err != nil {
		return errors.Errorf("error creating storage account client: %w", err)
	}

	exists, _, err := saClient.StorageAccountExists(ctx)
	if err != nil {
		return errors.Errorf("error checking if storage account exists: %w", err)
	}

	if !exists {
		return errors.Errorf("storage account %s does not exist in resource group %s",
			storageAccountName, resourceGroupName)
	}

	logInfo(l, "Verified storage account %s exists", storageAccountName)

	return nil
}

// getEndpointSuffix returns the endpoint suffix from config or derives it from cloud environment.
func getEndpointSuffix(config map[string]interface{}) string {
	if endpointSuffix, ok := config["endpoint_suffix"].(string); ok && endpointSuffix != "" {
		return endpointSuffix
	}

	if cloudEnv, ok := config["cloud_environment"].(string); ok && cloudEnv != "" {
		return azureauth.GetEndpointSuffix(cloudEnv)
	}

	return "core.windows.net" // Default to public cloud
}

// createBlobClientWithAuth creates an Azure blob client using the provided authentication result.
func createBlobClientWithAuth(url string, authResult *azureauth.AuthResult) (*azblob.Client, error) {
	if authResult.Method == azureauth.AuthMethodSasToken {
		sas := strings.TrimPrefix(authResult.SasToken, "?")

		return azblob.NewClientWithNoCredential(url+"?"+sas, nil)
	}

	return azblob.NewClient(url, authResult.Credential, nil)
}

// handleClientCreationError wraps client creation errors with appropriate context.
func handleClientCreationError(err error, storageAccountName string) error {
	if strings.Contains(err.Error(), "not exist") ||
		strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "dial tcp") {
		return errors.Errorf("storage account %s does not exist or is not accessible: %w",
			storageAccountName, err)
	}

	return errors.Errorf("error creating blob client with default credential: %w", err)
}

// handleConnectivityTestError processes errors from the connectivity test and returns appropriate error messages.
func handleConnectivityTestError(err error, l log.Logger, storageAccountName string) error {
	var respErr *azcore.ResponseError

	if !errors.As(err, &respErr) {
		return handleNonAzureConnectivityError(err, storageAccountName)
	}

	return handleAzureResponseError(respErr, l, storageAccountName)
}

// handleAzureResponseError processes Azure-specific response errors.
func handleAzureResponseError(respErr *azcore.ResponseError, l log.Logger, storageAccountName string) error {
	switch {
	case respErr.ErrorCode == "ContainerNotFound":
		logInfo(l, "Successfully verified storage account %s exists and is accessible", storageAccountName)

		return nil
	case respErr.StatusCode == http.StatusNotFound:
		return handleNotFoundError(respErr, l, storageAccountName)
	case respErr.StatusCode == http.StatusForbidden:
		return errors.Errorf("access denied to storage account %s: insufficient permissions (HTTP %d: %s). "+
			"Ensure you have 'Storage Blob Data Reader' or higher role assigned",
			storageAccountName, respErr.StatusCode, respErr.ErrorCode)
	case respErr.StatusCode == http.StatusUnauthorized:
		return errors.Errorf("authentication failed for storage account %s (HTTP %d: %s). "+
			"Check your Azure credentials and ensure they are valid",
			storageAccountName, respErr.StatusCode, respErr.ErrorCode)
	case respErr.StatusCode >= http.StatusInternalServerError:
		return errors.Errorf("Azure service error when accessing storage account %s (HTTP %d: %s). "+
			"This may be a temporary issue, please try again",
			storageAccountName, respErr.StatusCode, respErr.ErrorCode)
	default:
		return errors.Errorf("unexpected Azure API error when verifying storage account %s "+
			"(HTTP %d: %s)", storageAccountName, respErr.StatusCode, respErr.ErrorCode)
	}
}

// handleNotFoundError processes 404 errors to differentiate between storage account and container not found.
func handleNotFoundError(respErr *azcore.ResponseError, l log.Logger, storageAccountName string) error {
	if respErr.ErrorCode == "StorageAccountNotFound" || respErr.ErrorCode == "AccountNotFound" {
		return errors.Errorf("storage account %s does not exist (HTTP %d: %s)",
			storageAccountName, respErr.StatusCode, respErr.ErrorCode)
	}

	logInfo(l, "Successfully verified storage account %s exists (container not found as expected)", storageAccountName)

	return nil
}

// handleNonAzureConnectivityError processes non-Azure connectivity errors.
func handleNonAzureConnectivityError(err error, storageAccountName string) error {
	errMsg := err.Error()
	if strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "dial tcp") ||
		strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection timeout") {
		return errors.Errorf("storage account %s does not exist or is not accessible "+
			"(network error): %w", storageAccountName, err)
	}

	return errors.Errorf("unexpected error verifying access to storage account %s: %w",
		storageAccountName, err)
}

// verifyStorageAccountConnectivity tests connectivity to the storage account by attempting to access a test container.
func verifyStorageAccountConnectivity(ctx context.Context, l log.Logger, client *azblob.Client, storageAccountName string) error {
	testContainerName := "terragrunt-connectivity-test"
	testContainer := client.ServiceClient().NewContainerClient(testContainerName)

	_, err := testContainer.GetProperties(ctx, nil)
	if err != nil {
		return handleConnectivityTestError(err, l, storageAccountName)
	}

	return nil
}

// CreateBlobServiceClient creates a new Azure Blob Service client using the configuration from the backend.
func CreateBlobServiceClient(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, config map[string]interface{}) (*BlobServiceClient, error) {
	storageAccountName, okStorageAccountName := config["storage_account_name"].(string)
	if !okStorageAccountName || storageAccountName == "" {
		return nil, errors.Errorf("storage_account_name is required")
	}

	resourceGroupName, _ := config["resource_group_name"].(string)
	subscriptionID, _ := config["subscription_id"].(string)

	skipExistenceCheck := false
	if v, ok := config["skip_storage_account_existence_check"].(bool); ok {
		skipExistenceCheck = v
	}

	// Verify storage account exists via Management API if we have the required info
	if subscriptionID != "" && resourceGroupName != "" && !skipExistenceCheck {
		if err := verifyStorageAccountExists(ctx, l, config, storageAccountName, resourceGroupName); err != nil {
			return nil, err
		}
	}

	// Build the blob service URL
	endpointSuffix := getEndpointSuffix(config)
	url := fmt.Sprintf("https://%s.blob.%s", storageAccountName, endpointSuffix)

	// Get authentication credentials
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, errors.Errorf("error getting azure auth config: %w", err)
	}

	authResult, err := azureauth.GetTokenCredential(ctx, l, authConfig)
	if err != nil {
		return nil, errors.Errorf("error getting azure credentials: %w", err)
	}

	// Create the blob client
	client, err := createBlobClientWithAuth(url, authResult)
	if err != nil {
		return nil, handleClientCreationError(err, storageAccountName)
	}

	// Verify connectivity unless explicitly skipped
	if !skipExistenceCheck {
		if err := verifyStorageAccountConnectivity(ctx, l, client, storageAccountName); err != nil {
			return nil, err
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
			// Handle both ContainerNotFound and ResourceNotFound error codes
			// ResourceNotFound can occur when the container doesn't exist
			if respErr.ErrorCode == "ContainerNotFound" || respErr.ErrorCode == "ResourceNotFound" {
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
		logInfo(l, "Creating Azure Storage container %s", containerName)

		// Retry logic for ResourceNotFound errors which can occur when storage account
		// data plane is not yet fully provisioned after creation
		maxRetries := 3
		retryDelay := 5 * time.Second

		var lastErr error

		for attempt := 1; attempt <= maxRetries; attempt++ {
			_, err = c.client.CreateContainer(ctx, containerName, nil)
			if err == nil {
				return nil
			}

			lastErr = err

			// Check if this is a ResourceNotFound error (storage account data plane not ready)
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.ErrorCode == "ResourceNotFound" {
				if attempt < maxRetries {
					logInfo(l, "Storage account data plane not yet ready (attempt %d/%d), retrying in %v...",
						attempt, maxRetries, retryDelay)

					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(retryDelay):
						// Continue to next attempt
					}

					continue
				}
			}

			// Non-retryable error or max retries reached
			break
		}

		return NewContainerCreationError(lastErr, containerName)
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

// NewContainerCreationError creates a new ContainerCreationError preserving the original error chain.
func NewContainerCreationError(underlying error, containerName string) ContainerCreationError {
	return ContainerCreationError{
		Underlying:    underlying,
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
