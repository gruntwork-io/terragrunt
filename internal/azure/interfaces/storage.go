// Package interfaces provides interface definitions for Azure services used by Terragrunt
package interfaces

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Account configuration constants
const (
	KindStorageV2        types.AccountKind = "StorageV2"
	KindStorage          types.AccountKind = "Storage"
	KindBlockBlobStorage types.AccountKind = "BlockBlobStorage"
)

const (
	TierStandard types.AccountTier = "Standard"
	TierPremium  types.AccountTier = "Premium"
)

const (
	TierHot     types.AccessTier = "Hot"
	TierCool    types.AccessTier = "Cool"
	TierArchive types.AccessTier = "Archive"
)

const (
	RAGRS types.ReplicationType = "RAGRS"
	GRS   types.ReplicationType = "GRS"
	LRS   types.ReplicationType = "LRS"
	ZRS   types.ReplicationType = "ZRS"
)

// StorageAccountService defines the interface for Azure Storage Account operations
type StorageAccountService interface {
	CreateStorageAccount(ctx context.Context, cfg *types.StorageAccountConfig) error
	DeleteStorageAccount(ctx context.Context, resourceGroupName, accountName string) error
	GetStorageAccount(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccount, error)
	GetStorageAccountKeys(ctx context.Context, resourceGroupName, accountName string) ([]string, error)
	GetStorageAccountSAS(ctx context.Context, resourceGroupName, accountName string) (string, error)
	GetStorageAccountProperties(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccountProperties, error)
	IsVersioningEnabled(ctx context.Context) (bool, error)
}

// BlobService defines the interface for Azure Blob Storage operations
type BlobService interface {
	// Blob Operations
	GetObject(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error)

	// Container Operations
	ContainerExists(ctx context.Context, containerName string) (bool, error)
	CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error
	DeleteContainer(ctx context.Context, l log.Logger, containerName string) error

	// Blob Management
	DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error
	UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error
	CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient BlobService, dstContainer, dstKey string) error
}

// ResourceGroupService defines the interface for Azure Resource Group operations
type ResourceGroupService interface {
	// Resource Group Management
	EnsureResourceGroup(ctx context.Context, l log.Logger, resourceGroupName, location string, tags map[string]string) error
	ResourceGroupExists(ctx context.Context, resourceGroupName string) (bool, error)
	DeleteResourceGroup(ctx context.Context, l log.Logger, resourceGroupName string) error

	// Resource Group Information
	GetResourceGroup(ctx context.Context, resourceGroupName string) (*armresources.ResourceGroup, error)
}

// ResourceNotFoundError represents an error for a resource that wasn't found
type ResourceNotFoundError struct {
	ResourceType string
	Name         string
	Message      string
}

func (e *ResourceNotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	return "Resource not found: " + e.ResourceType + " " + e.Name
}

// NewResourceNotFoundError creates a new ResourceNotFoundError
func NewResourceNotFoundError(resourceType, name string) *ResourceNotFoundError {
	return &ResourceNotFoundError{
		ResourceType: resourceType,
		Name:         name,
	}
}
