// Package factory provides factory functions for creating Azure service implementations
package factory

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Adapter implementations to bridge between azurehelper and interfaces

// storageAccountServiceAdapter implements interfaces.StorageAccountService
type storageAccountServiceAdapter struct {
	client *azurehelper.StorageAccountClient
}

// Storage Account Management

// CreateStorageAccount creates a new storage account using the new types config
func (s *storageAccountServiceAdapter) CreateStorageAccount(ctx context.Context, cfg *types.StorageAccountConfig) error {
	// Convert the types.StorageAccountConfig to azurehelper.StorageAccountConfig
	azureConfig := azurehelper.StorageAccountConfig{
		StorageAccountName:    cfg.Name,
		ResourceGroupName:     cfg.ResourceGroupName,
		Location:              cfg.Location,
		EnableVersioning:      cfg.EnableVersioning,
		AllowBlobPublicAccess: cfg.AllowBlobPublicAccess,
		AccountKind:           string(cfg.AccountKind),
		AccountTier:           string(cfg.AccountTier),
		AccessTier:            string(cfg.AccessTier),
		ReplicationType:       string(cfg.ReplicationType),
		Tags:                  cfg.Tags,
	}

	// Use CreateStorageAccountIfNecessary since there's no direct Create method
	logger := log.Default()
	return s.client.CreateStorageAccountIfNecessary(ctx, logger, azureConfig)
}

// DeleteStorageAccount deletes a storage account by resource group and account name
func (s *storageAccountServiceAdapter) DeleteStorageAccount(ctx context.Context, resourceGroupName, accountName string) error {
	logger := log.Default()
	return s.client.DeleteStorageAccount(ctx, logger)
}

// GetStorageAccount retrieves storage account information
func (s *storageAccountServiceAdapter) GetStorageAccount(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccount, error) {
	// Use StorageAccountExists as it returns the account object
	_, account, err := s.client.StorageAccountExists(ctx)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, nil
	}

	// Convert ARM storage account to our types
	result := &types.StorageAccount{
		Name:              accountName,
		ResourceGroupName: resourceGroupName,
		Location:          *account.Location,
	}

	if account.Properties != nil {
		result.Properties = &types.StorageAccountProperties{
			ProvisioningState: string(*account.Properties.ProvisioningState),
		}
		if account.Properties.EnableHTTPSTrafficOnly != nil {
			result.Properties.SupportsHttpsOnly = *account.Properties.EnableHTTPSTrafficOnly
		}
		if account.Properties.AccessTier != nil {
			result.Properties.AccessTier = types.AccessTier(*account.Properties.AccessTier)
		}
		if account.Properties.IsHnsEnabled != nil {
			result.Properties.IsHnsEnabled = *account.Properties.IsHnsEnabled
		}
	}

	return result, nil
}

// GetStorageAccountKeys retrieves storage account keys
func (s *storageAccountServiceAdapter) GetStorageAccountKeys(ctx context.Context, resourceGroupName, accountName string) ([]string, error) {
	// The azurehelper client doesn't expose this method yet
	// This would need to be implemented in the azurehelper client
	// For now, return a placeholder implementation
	return []string{}, nil
}

// GetStorageAccountSAS generates a SAS token for the storage account
func (s *storageAccountServiceAdapter) GetStorageAccountSAS(ctx context.Context, resourceGroupName, accountName string) (string, error) {
	// This would need to be implemented in the azurehelper client
	// For now, return a placeholder implementation
	return "", nil
}

// GetStorageAccountProperties retrieves properties of a storage account
func (s *storageAccountServiceAdapter) GetStorageAccountProperties(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccountProperties, error) {
	account, err := s.GetStorageAccount(ctx, resourceGroupName, accountName)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, nil
	}
	return account.Properties, nil
}

// Exists checks if a storage account exists
func (s *storageAccountServiceAdapter) Exists(ctx context.Context, config types.StorageAccountConfig) (bool, error) {
	exists, _, err := s.client.StorageAccountExists(ctx)
	return exists, err
}

// Create creates a new storage account
func (s *storageAccountServiceAdapter) Create(ctx context.Context, config types.StorageAccountConfig) error {
	return s.CreateStorageAccount(ctx, &config)
}

// Delete deletes a storage account
func (s *storageAccountServiceAdapter) Delete(ctx context.Context, l log.Logger) error {
	return s.client.DeleteStorageAccount(ctx, l)
}

// Versioning Management

// GetStorageAccountVersioning gets the versioning state of the storage account
func (s *storageAccountServiceAdapter) GetStorageAccountVersioning(ctx context.Context) (bool, error) {
	return s.client.GetStorageAccountVersioning(ctx)
}

// IsVersioningEnabled checks if blob versioning is enabled for the storage account
func (s *storageAccountServiceAdapter) IsVersioningEnabled(ctx context.Context) (bool, error) {
	return s.client.GetStorageAccountVersioning(ctx)
}

// EnableStorageAccountVersioning enables versioning on the storage account
func (s *storageAccountServiceAdapter) EnableStorageAccountVersioning(ctx context.Context, l log.Logger) error {
	return s.client.EnableStorageAccountVersioning(ctx, l)
}

// DisableStorageAccountVersioning disables versioning on the storage account
func (s *storageAccountServiceAdapter) DisableStorageAccountVersioning(ctx context.Context, l log.Logger) error {
	return s.client.DisableStorageAccountVersioning(ctx, l)
}

// Resource Group Management

// EnsureResourceGroup ensures a resource group exists
func (s *storageAccountServiceAdapter) EnsureResourceGroup(ctx context.Context, l log.Logger, location string) error {
	return s.client.EnsureResourceGroup(ctx, l, location)
}

// Resource Information

// GetResourceID gets the resource ID of the storage account
func (s *storageAccountServiceAdapter) GetResourceID(ctx context.Context) string {
	// Get the storage account details first
	_, account, err := s.client.StorageAccountExists(ctx)
	if err != nil || account == nil {
		return ""
	}

	// Return the ID from the account object
	if account.ID != nil {
		return *account.ID
	}
	return ""
}

// Utility

// IsPermissionError checks if an error is a permission error
func (s *storageAccountServiceAdapter) IsPermissionError(err error) bool {
	return s.client.IsPermissionError(err)
}

// blobServiceAdapter implements interfaces.BlobService
type blobServiceAdapter struct {
	client *azurehelper.BlobServiceClient
}

// Blob Operations

// GetObject gets a blob using the new types
func (b *blobServiceAdapter) GetObject(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error) {
	// Convert types to azurehelper types
	azureInput := &azurehelper.GetObjectInput{
		Container: &input.ContainerName,
		Key:       &input.BlobName,
	}

	azureOutput, err := b.client.GetObject(ctx, azureInput)
	if err != nil {
		return nil, err
	}

	// Read the body into a byte slice
	content, err := io.ReadAll(azureOutput.Body)
	if err != nil {
		return nil, err
	}
	defer azureOutput.Body.Close()

	// Convert back to our types
	return &types.GetObjectOutput{
		Content:    content,
		Properties: make(map[string]string), // Empty for now, could be populated with metadata
	}, nil
}

// Container Operations

// ContainerExists checks if a container exists
func (b *blobServiceAdapter) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	return b.client.ContainerExists(ctx, containerName)
}

// CreateContainerIfNecessary creates a container if it doesn't exist
func (b *blobServiceAdapter) CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	return b.client.CreateContainerIfNecessary(ctx, l, containerName)
}

// DeleteContainer deletes a container
func (b *blobServiceAdapter) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	return b.client.DeleteContainer(ctx, l, containerName)
}

// Blob Management

// DeleteBlobIfNecessary deletes a blob if it exists
func (b *blobServiceAdapter) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	return b.client.DeleteBlobIfNecessary(ctx, l, containerName, blobName)
}

// UploadBlob uploads a blob
func (b *blobServiceAdapter) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	return b.client.UploadBlob(ctx, l, containerName, blobName, data)
}

// CopyBlobToContainer copies a blob to another container
func (b *blobServiceAdapter) CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient interfaces.BlobService, dstContainer, dstKey string) error {
	// This is a more complex operation that would need to be implemented
	// For now, return a placeholder implementation that indicates it's not implemented
	return fmt.Errorf("CopyBlobToContainer is not yet implemented in the adapter")
}

// resourceGroupServiceAdapter implements interfaces.ResourceGroupService
type resourceGroupServiceAdapter struct {
	client *azurehelper.ResourceGroupClient
}

// Resource Group Management

// EnsureResourceGroup ensures a resource group exists
func (r *resourceGroupServiceAdapter) EnsureResourceGroup(ctx context.Context, l log.Logger, resourceGroupName, location string, tags map[string]string) error {
	return r.client.EnsureResourceGroup(ctx, l, resourceGroupName, location, tags)
}

// ResourceGroupExists checks if a resource group exists
func (r *resourceGroupServiceAdapter) ResourceGroupExists(ctx context.Context, resourceGroupName string) (bool, error) {
	return r.client.ResourceGroupExists(ctx, resourceGroupName)
}

// DeleteResourceGroup deletes a resource group
func (r *resourceGroupServiceAdapter) DeleteResourceGroup(ctx context.Context, l log.Logger, resourceGroupName string) error {
	return r.client.DeleteResourceGroup(ctx, l, resourceGroupName)
}

// Resource Group Information

// GetResourceGroup gets a resource group
func (r *resourceGroupServiceAdapter) GetResourceGroup(ctx context.Context, resourceGroupName string) (*armresources.ResourceGroup, error) {
	return r.client.GetResourceGroup(ctx, resourceGroupName)
}
