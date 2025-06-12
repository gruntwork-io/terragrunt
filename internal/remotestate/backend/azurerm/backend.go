// Package azurerm represents Azure storage backend for remote state
package azurerm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

var _ backend.Backend = new(Backend)

// Backend implements the backend interface for the Azure backend
type Backend struct {
	*backend.CommonBackend
}

// NewBackend creates a new Azure backend
func NewBackend() *Backend {
	return &Backend{CommonBackend: backend.NewCommonBackend(BackendName)}
}

// Bootstrap creates the Azure Storage container if it doesn't exist
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	// Validate authentication methods, prioritizing Azure AD auth
	hasAzureAD := azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth
	hasMSI := azureCfg.RemoteStateConfigAzurerm.UseMsi
	hasServicePrincipal := azureCfg.RemoteStateConfigAzurerm.ClientID != "" && azureCfg.RemoteStateConfigAzurerm.ClientSecret != "" &&
		azureCfg.RemoteStateConfigAzurerm.TenantID != "" && azureCfg.RemoteStateConfigAzurerm.SubscriptionID != ""
	hasSasToken := azureCfg.RemoteStateConfigAzurerm.SasToken != ""

	// Legacy/Deprecated auth methods
	hasKeyAuth := azureCfg.RemoteStateConfigAzurerm.ConnectionString != "" || azureCfg.RemoteStateConfigAzurerm.StorageAccountKey != ""

	// Check environment variables if no explicit credentials in config
	hasEnvCreds := false

	if !hasAzureAD && !hasMSI && !hasServicePrincipal && !hasSasToken && !hasKeyAuth {
		// Check for service principal environment variables first
		// Check all required service principal environment variables
		envClientID := os.Getenv("AZURE_CLIENT_ID")
		envClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
		envTenantID := os.Getenv("AZURE_TENANT_ID")
		envSubID := os.Getenv("AZURE_SUBSCRIPTION_ID")

		if envClientID != "" && envClientSecret != "" && envTenantID != "" && envSubID != "" {
			hasServicePrincipal, hasEnvCreds = true, true
		}

		envSas := os.Getenv("AZURE_STORAGE_SAS_TOKEN")
		if envSas != "" {
			hasSasToken, hasEnvCreds = true, true
		}

		// Legacy/Deprecated environment variables - show deprecation warning
		envKey := os.Getenv("ARM_ACCESS_KEY")
		if envKey != "" {
			l.Warn("Using ARM_ACCESS_KEY is deprecated. Please switch to Azure AD authentication.")

			hasKeyAuth, hasEnvCreds = true, true
		} else if envKey := os.Getenv("AZURE_STORAGE_KEY"); envKey != "" {
			l.Warn("Using AZURE_STORAGE_KEY is deprecated. Please switch to Azure AD authentication.")

			hasKeyAuth, hasEnvCreds = true, true
		}
	}

	if hasKeyAuth {
		l.Warn("Using storage account key authentication is deprecated. Please switch to Azure AD authentication.")
	}

	if !hasAzureAD && !hasMSI && !hasServicePrincipal && !hasSasToken && !hasKeyAuth && !hasEnvCreds {
		return errors.New("no valid authentication method found: Azure AD auth is recommended. Alternatively, provide one of: MSI, service principal credentials, or SAS token")
	}

	// ensure that only one goroutine can initialize storage
	mu := backend.GetBucketMutex(azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	mu.Lock()
	defer mu.Unlock()

	if backend.IsConfigInited(azureCfg) {
		l.Debugf("%s storage account %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	err = client.CreateContainerIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
	if err != nil {
		return err
	}

	backend.MarkConfigInited(azureCfg)

	return nil
}

// NeedsBootstrap checks if Azure Storage container needs initialization
func (backend *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return false, err
	}

	// Skip initialization if marked as already initialized
	if backend.IsConfigInited(azureCfg) {
		return false, nil
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return false, err
	}

	containerExists, existsErr := client.ContainerExists(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)

	if existsErr != nil {
		return false, existsErr
	}

	if !containerExists {
		return true, nil
	}

	return false, nil
}

// Delete deletes the remote state file from Azure Storage
func (backend *Backend) Delete(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s blob %s will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)

	shouldContinue, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	}

	if !shouldContinue {
		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	return client.DeleteBlobIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)
}

// DeleteBucket deletes the entire Azure Storage container
func (backend *Backend) DeleteBucket(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s and all its contents will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName)

	shouldContinue, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	}

	if !shouldContinue {
		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(l, opts, backendConfig)
	if err != nil {
		return err
	}

	return client.DeleteContainer(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
}

// Migrate copies the state file from source container to destination container and deletes the original
func (backend *Backend) Migrate(ctx context.Context, l log.Logger, srcBackendConfig, dstBackendConfig backend.Config, opts *options.TerragruntOptions) error {
	// If not using force flag, warn about versioning being a storage account level setting
	if !opts.ForceBackendMigrate {
		l.Warn("Warning: Blob versioning in Azure Storage is a storage account level setting. Use the Azure Portal or CLI to verify that blob versioning is enabled on both source and destination storage accounts.")
	}

	srcCfg, err := Config(srcBackendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	dstCfg, err := Config(dstBackendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	srcClient, err := azurehelper.CreateBlobServiceClient(l, opts, srcBackendConfig)
	if err != nil {
		return fmt.Errorf("error creating source blob client: %w", err)
	}

	dstClient, err := azurehelper.CreateBlobServiceClient(l, opts, dstBackendConfig)
	if err != nil {
		return fmt.Errorf("error creating destination blob client: %w", err)
	}

	// Check that source container exists and state file is present
	srcContainer := srcCfg.RemoteStateConfigAzurerm.ContainerName
	srcKey := srcCfg.RemoteStateConfigAzurerm.Key

	exists, existsErr := srcClient.ContainerExists(ctx, srcContainer)
	if existsErr != nil {
		return fmt.Errorf("error checking source container existence: %w", existsErr)
	}

	if !exists {
		return fmt.Errorf("source container %s does not exist", srcContainer)
	}

	// Ensure destination container exists (create if necessary)
	dstContainer := dstCfg.RemoteStateConfigAzurerm.ContainerName

	createErr := dstClient.CreateContainerIfNecessary(ctx, l, dstContainer)
	if createErr != nil {
		return fmt.Errorf("error creating destination container: %w", createErr)
	}

	// Copy state file from source to destination
	err = srcClient.CopyBlobToContainer(ctx, srcContainer, srcKey, dstClient, dstContainer, dstCfg.RemoteStateConfigAzurerm.Key)
	if err != nil {
		return fmt.Errorf("error copying state file: %w", err)
	}

	// Verify the copy succeeded by reading the destination blob
	dstInput := &azurehelper.GetObjectInput{
		Bucket: &dstContainer,
		Key:    &dstCfg.RemoteStateConfigAzurerm.Key,
	}

	_, getObjectErr := dstClient.GetObject(ctx, dstInput)
	if getObjectErr != nil {
		return fmt.Errorf("error verifying destination state file: %w", getObjectErr)
	}

	// Delete the source state file
	deleteErr := srcClient.DeleteBlobIfNecessary(ctx, l, srcContainer, srcKey)
	if deleteErr != nil {
		return fmt.Errorf("error deleting source state file: %w", deleteErr)
	}

	return nil
}

// GetTFInitArgs returns the subset of config to pass to terraform init
func (backend *Backend) GetTFInitArgs(backendConfig backend.Config) map[string]any {
	// Azure backend takes all config values and filters out terragruntOnly ones
	return Config(backendConfig).FilterOutTerragruntKeys()
}
