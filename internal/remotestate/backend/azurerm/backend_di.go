// Package azurerm provides dependency injection support for Azure backend
package azurerm

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/azure/factory"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// AzureBackendDependencies holds the injected Azure service dependencies
type AzureBackendDependencies struct {
	StorageAccountService interfaces.StorageAccountService
	BlobService           interfaces.BlobService
	ResourceGroupService  interfaces.ResourceGroupService
}

// NewAzureBackendDependencies creates a new set of Azure backend dependencies
func NewAzureBackendDependencies(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	config map[string]interface{},
	subscriptionID string,
) (*AzureBackendDependencies, error) {
	azureFactory := factory.NewAzureServiceFactory()

	storageAccountService, err := azureFactory.GetStorageAccountService(ctx, l, config)
	if err != nil {
		return nil, err
	}

	blobService, err := azureFactory.GetBlobService(ctx, l, config)
	if err != nil {
		return nil, err
	}

	resourceGroupService, err := azureFactory.GetResourceGroupService(ctx, l, config)
	if err != nil {
		return nil, err
	}

	return &AzureBackendDependencies{
		StorageAccountService: storageAccountService,
		BlobService:           blobService,
		ResourceGroupService:  resourceGroupService,
	}, nil
}

// Example of how to update the Backend struct to use dependency injection
// This would replace the existing Backend struct:
//
// type Backend struct {
//     *RemoteStateConfigAzurerm
//     azureDeps *AzureBackendDependencies
// }
//
// Example usage in the backend methods:
//
// func (backend *Backend) bootstrapStorageAccount(ctx context.Context, l log.Logger, config StorageAccountConfig) error {
//     // Instead of: client, err := azurehelper.CreateStorageAccountClient(ctx, l, backendConfig)
//     // Use: return backend.azureDeps.StorageAccountService.CreateStorageAccountIfNecessary(ctx, l, config)
//     return backend.azureDeps.StorageAccountService.CreateStorageAccountIfNecessary(ctx, l, config)
// }
//
// func (backend *Backend) GetFile(ctx context.Context, l log.Logger, backendConfig map[string]interface{}, remoteStateFile string) (io.ReadCloser, error) {
//     // Instead of: client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, backendConfig)
//     // Use: return backend.azureDeps.BlobService.GetObject(ctx, &azurehelper.GetObjectInput{...})
//     input := &azurehelper.GetObjectInput{
//         Container: &backendConfig["container_name"].(string),
//         Key:       &remoteStateFile,
//     }
//     output, err := backend.azureDeps.BlobService.GetObject(ctx, input)
//     if err != nil {
//         return nil, err
//     }
//     return output.Body, nil
// }
