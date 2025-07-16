// Package implementations provides production implementations of Azure service interfaces
package implementations

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// BlobServiceImpl is the production implementation of BlobService
type BlobServiceImpl struct {
	client *azurehelper.BlobServiceClient
}

// NewBlobService creates a new BlobService implementation
func NewBlobService(client *azurehelper.BlobServiceClient) interfaces.BlobService {
	return &BlobServiceImpl{
		client: client,
	}
}

// GetObject gets a blob object using the new types
func (b *BlobServiceImpl) GetObject(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}

	if input.ContainerName == "" {
		return nil, fmt.Errorf("container name is required")
	}

	if input.BlobName == "" {
		return nil, fmt.Errorf("blob name is required")
	}

	// Convert types.GetObjectInput to azurehelper.GetObjectInput
	helperInput := &azurehelper.GetObjectInput{
		Container: to.Ptr(input.ContainerName),
		Key:       to.Ptr(input.BlobName),
	}

	// Call the azurehelper method
	helperOutput, err := b.client.GetObject(ctx, helperInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get blob object: %w", err)
	}

	if helperOutput == nil {
		return nil, fmt.Errorf("helper returned nil output")
	}

	if helperOutput.Body == nil {
		return nil, fmt.Errorf("helper returned nil body")
	}

	// Read the content from the ReadCloser
	content, err := io.ReadAll(helperOutput.Body)
	if err != nil {
		helperOutput.Body.Close()
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}

	// Close the ReadCloser
	if err := helperOutput.Body.Close(); err != nil {
		return nil, fmt.Errorf("failed to close blob stream: %w", err)
	}

	// Convert azurehelper.GetObjectOutput to types.GetObjectOutput
	output := &types.GetObjectOutput{
		Content:    content,
		Properties: make(map[string]string), // Initialize empty properties for now
	}

	return output, nil
}

// ContainerExists checks if a container exists
func (b *BlobServiceImpl) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	return b.client.ContainerExists(ctx, containerName)
}

// CreateContainerIfNecessary creates a container if it doesn't exist
func (b *BlobServiceImpl) CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	return b.client.CreateContainerIfNecessary(ctx, l, containerName)
}

// DeleteContainer deletes a container
func (b *BlobServiceImpl) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	return b.client.DeleteContainer(ctx, l, containerName)
}

// DeleteBlobIfNecessary deletes a blob if it exists
func (b *BlobServiceImpl) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	return b.client.DeleteBlobIfNecessary(ctx, l, containerName, blobName)
}

// UploadBlob uploads a blob to a container
func (b *BlobServiceImpl) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	return b.client.UploadBlob(ctx, l, containerName, blobName, data)
}

// CopyBlobToContainer copies a blob from one container to another
func (b *BlobServiceImpl) CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient interfaces.BlobService, dstContainer, dstKey string) error {
	// First, get the source blob
	input := &types.GetObjectInput{
		ContainerName: srcContainer,
		BlobName:      srcKey,
	}

	output, err := b.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to get source blob: %w", err)
	}

	// Upload the blob to the destination
	err = dstClient.UploadBlob(ctx, nil, dstContainer, dstKey, output.Content)
	if err != nil {
		return fmt.Errorf("failed to upload blob to destination: %w", err)
	}

	return nil
}
