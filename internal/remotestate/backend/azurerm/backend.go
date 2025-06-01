// Package azurerm represents Azure storage backend for interacting with	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return false, err
	}

	containerExists, err := client.ContainerExists(ctx, azureCfg.ContainerName)
	if err != nil {
		return false, err
	}tate.
package azurerm

import (
	"context"
	"fmt"
	"path"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

var _ backend.Backend = new(Backend)

type Backend struct {
	*backend.CommonBackend
}

func NewBackend() *Backend {
	return &Backend{CommonBackend: backend.NewCommonBackend(BackendName)}
}

// Bootstrap creates the Azure Storage container if it doesn't exist and enables versioning if requested
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	// ensure that only one goroutine can initialize storage
	mu := backend.GetBucketMutex(azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	mu.Lock()
	defer mu.Unlock()

	if backend.IsConfigInited(azureCfg.RemoteStateConfigAzurerm.CacheKey()) {
		l.Debugf("%s storage account %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	if err := client.CreateContainerIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName); err != nil {
		return err
	}

	if !azureCfg.SkipBlobVersioning {
		if err := client.EnableVersioningIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName); err != nil {
			return err
		}
	}

	backend.MarkConfigInited(azureCfg.RemoteStateConfigAzurerm.CacheKey())
	return nil
}

// NeedsBootstrap checks if Azure Storage container needs initialization
func (backend *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return false, err
	}

	// Skip initialization if marked as already initialized
	if backend.IsConfigInited(azureCfg.RemoteStateConfigAzurerm.CacheKey()) {
		return false, nil
	}

	client, err := awshelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return false, err
	}

	containerExists, err := client.ContainerExists(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)
	if err != nil {
		return false, err
	}

	if !containerExists {
		return true, nil
	}

	if !azureCfg.SkipBlobVersioning {
		versioningEnabled, err := client.IsVersioningEnabled(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		if err != nil {
			return false, err
		}

		if !versioningEnabled {
			return true, nil
		}
	}

	return false, nil
}

// Delete deletes the remote state file from Azure Storage
func (backend *Backend) Delete(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s blob %s will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)
	if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteBlobIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)
	}

	return nil
}

// DeleteBucket deletes the entire Azure Storage container
func (backend *Backend) DeleteBucket(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s and all its contents will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName)
	if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteContainer(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
	}

	return nil
}

// GetTFInitArgs returns the subset of config to pass to terraform init
func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return config.FilterOutTerragruntKeys()
}
