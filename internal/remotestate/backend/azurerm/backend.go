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

// BackendName is the name of the Azure backend
const BackendName = "azurerm"
var _ backend.Backend = new(Backend)

// Backend implements the backend interface for the Azure backend.
type Backend struct {
	*backend.CommonBackend
}

// NewBackend creates a new Azure backend.
func NewBackend() *Backend {
	return &Backend{CommonBackend: backend.NewCommonBackend(BackendName)}
}

// Bootstrap creates the Azure Storage container if it doesn't exist.
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}
	
	// Check upfront if storage account creation is requested and validate required fields
	if createIfNotExists, ok := backendConfig["create_storage_account_if_not_exists"].(bool); ok && createIfNotExists {
		subscriptionID, hasSubscription := backendConfig["subscription_id"].(string)
		if !hasSubscription || subscriptionID == "" {
			return fmt.Errorf("subscription_id is required for storage account creation")
		}
		
		location, hasLocation := backendConfig["location"].(string) 
		if !hasLocation || location == "" {
			return fmt.Errorf("location is required for storage account creation")
		}
	}

	// Check for authentication methods - Azure AD auth is now required
	// Set default to Azure AD auth if not explicitly set
	if !azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth {
		azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth = true
		backendConfig["use_azuread_auth"] = true
		l.Info("Azure AD authentication is now the default and required authentication method")
	}
	
	hasAzureAD := azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth
	hasMSI := azureCfg.RemoteStateConfigAzurerm.UseMsi
	hasServicePrincipal := azureCfg.RemoteStateConfigAzurerm.ClientID != "" && azureCfg.RemoteStateConfigAzurerm.ClientSecret != "" &&
		azureCfg.RemoteStateConfigAzurerm.TenantID != "" && azureCfg.RemoteStateConfigAzurerm.SubscriptionID != ""
	hasSasToken := azureCfg.RemoteStateConfigAzurerm.SasToken != ""

	// Check environment variables if no explicit credentials in config
	hasEnvCreds := false

	if !hasAzureAD && !hasMSI && !hasServicePrincipal && !hasSasToken {
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
		if envKey := os.Getenv("ARM_ACCESS_KEY"); envKey != "" {
			l.Warn("ARM_ACCESS_KEY is no longer supported. Please switch to Azure AD authentication.")
			hasEnvCreds = true
		} else if envKey := os.Getenv("AZURE_STORAGE_KEY"); envKey != "" {
			l.Warn("AZURE_STORAGE_KEY is no longer supported. Please switch to Azure AD authentication.")
			hasEnvCreds = true
		}
	}

	// Always use Azure AD auth - if we detect legacy key env vars, we still warn but ignore them
	l.Debug("Using Azure AD authentication")

	if !hasAzureAD && !hasMSI && !hasServicePrincipal && !hasSasToken && !hasEnvCreds {
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

	// Check if we need to handle storage account creation/validation
	if azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
		// Validate required fields before attempting any Azure operations
		if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
			return fmt.Errorf("subscription_id is required for storage account creation")
		}
		
		if azureCfg.StorageAccountConfig.Location == "" {
			return fmt.Errorf("location is required for storage account creation")
		}
		
		err = backend.bootstrapStorageAccount(ctx, l, opts, azureCfg, backendConfig)
		if err != nil {
			return fmt.Errorf("error bootstrapping storage account: %w", err)
		}
	}

	// Create the container if necessary
	err = client.CreateContainerIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
	if err != nil {
		return err
	}

	backend.MarkConfigInited(azureCfg)

	return nil
}

// NeedsBootstrap checks if Azure Storage container needs initialization.
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

	// Check if storage account bootstrap is requested
	if azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
		// We will always return true if CreateStorageAccountIfNotExists is true
		// The actual check will be done in Bootstrap() to reduce duplicate API calls
		return true, nil
	}

	// Check if container exists
	containerExists, existsErr := client.ContainerExists(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)

	if existsErr != nil {
		// Try to convert to Azure error
		azureErr := azurehelper.ConvertAzureError(existsErr)
		
		// If the storage account doesn't exist, we need bootstrap
		if azureErr != nil && (azureErr.StatusCode == 404 || azureErr.ErrorCode == "StorageAccountNotFound") {
			return true, nil
		}
		
		return false, existsErr
	}

	if !containerExists {
		return true, nil
	}

	return false, nil
}

// Delete deletes the remote state file from Azure Storage.
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

// DeleteBucket deletes the entire Azure Storage container.
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

// Migrate copies the state file from source container to destination container and deletes the original.
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

// DeleteStorageAccount deletes an Azure Storage account.
func (backend *Backend) DeleteStorageAccount(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		return err
	}

	var (
		storageAccountName = azureCfg.RemoteStateConfigAzurerm.StorageAccountName
		resourceGroupName  = azureCfg.StorageAccountConfig.ResourceGroupName
	)

	if resourceGroupName == "" {
		return fmt.Errorf("resource_group_name is required to delete a storage account")
	}

	if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
		return fmt.Errorf("subscription_id is required to delete a storage account")
	}

	// Check if we're in non-interactive mode
	if opts.NonInteractive {
		return fmt.Errorf("cannot delete storage account %s in non-interactive mode, user confirmation is required", storageAccountName)
	}

	// Ask for confirmation
	prompt := fmt.Sprintf("Azure Storage Account %s will be completely deleted. All containers and blobs will be permanently deleted. Do you want to continue?", storageAccountName)
	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	} 
	
	if !yes {
		return nil
	}

	// Create config for storage account client
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"resource_group_name":  resourceGroupName,
		"subscription_id":      azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"use_azuread_auth":     true,
	}

	// Create storage account client
	client, err := azurehelper.CreateStorageAccountClient(ctx, l, opts, storageAccountConfig)
	if err != nil {
		return err
	}

	// Delete the storage account
	l.Infof("Deleting Azure Storage Account %s...", storageAccountName)
	return client.DeleteStorageAccount(ctx, l)
}

// bootstrapStorageAccount handles creating or checking a storage account
func (backend *Backend) bootstrapStorageAccount(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, azureCfg *ExtendedRemoteStateConfigAzurerm, backendConfig backend.Config) error {
	// Import the armstorage package conditionally
	// We need to add the armstorage package to go.mod
	// go get github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage

	// Validate that required fields are set for storage account operations
	if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
		return fmt.Errorf("subscription_id is required for storage account creation")
	}
	
	if azureCfg.StorageAccountConfig.Location == "" {
		return fmt.Errorf("location is required for storage account creation")
	}

	// For now, we'll check if the package is available
	l.Infof("Checking if storage account %s exists", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

	// Set the subscription ID
	subscriptionID := azureCfg.RemoteStateConfigAzurerm.SubscriptionID

	// Create a config that merges the backend config with the storage account config
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"subscription_id":      subscriptionID,
		"resource_group_name":  azureCfg.StorageAccountConfig.ResourceGroupName,
		"location":             azureCfg.StorageAccountConfig.Location,
		"use_azuread_auth":     azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth,
	}

	// Check if the storage account exists using the data plane client (blob service)
	// This is a workaround until we properly implement the storage account client
	blobClient, err := azurehelper.CreateBlobServiceClient(l, opts, storageAccountConfig)
	if err != nil {
		return fmt.Errorf("error creating blob service client: %w", err)
	}
	// Create a storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, l, opts, storageAccountConfig)
	if err != nil {
		return fmt.Errorf("error creating storage account client: %w", err)
	}
	
	// Convert configuration to the expected format for the storage account client
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        subscriptionID,
		ResourceGroupName:     azureCfg.StorageAccountConfig.ResourceGroupName,
		StorageAccountName:    azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		Location:              azureCfg.StorageAccountConfig.Location,
		EnableHierarchicalNS:  azureCfg.StorageAccountConfig.EnableHierarchicalNS,
		EnableVersioning:      azureCfg.StorageAccountConfig.EnableVersioning, 
		AllowBlobPublicAccess: azureCfg.StorageAccountConfig.AllowBlobPublicAccess,
		AccountKind:           azureCfg.StorageAccountConfig.AccountKind,
		AccountTier:           azureCfg.StorageAccountConfig.AccountTier,
		AccessTier:            azureCfg.StorageAccountConfig.AccessTier,
		ReplicationType:       azureCfg.StorageAccountConfig.ReplicationType,
		Tags:                  azureCfg.StorageAccountConfig.StorageAccountTags,
	}
	
	// If no tags provided, set default
	if len(saConfig.Tags) == 0 {
		saConfig.Tags = map[string]string{"created-by": "terragrunt"}
	}
	
	// Create resource group and storage account if needed
	err = storageClient.CreateStorageAccountIfNecessary(ctx, l, saConfig)
	if err != nil {
		return fmt.Errorf("error creating storage account: %w", err)
	}
	
	// Ensure the current user has Storage Blob Data Owner role
	// This is important for both new and existing storage accounts
	err = storageClient.AssignStorageBlobDataOwnerRole(ctx, l)
	if err != nil {
		l.Warnf("Failed to assign Storage Blob Data Owner role: %v", err)
		// Don't fail the entire process if role assignment fails
	}
	
	l.Infof("Storage account %s exists and is accessible", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	
	// For safety, try the blob client operation to confirm access
	exists, err := blobClient.ContainerExists(ctx, "_terragrunt_bootstrap_test")
	if err != nil {
		// Try to convert to Azure error
		azureErr := azurehelper.ConvertAzureError(err)
		
		// Check if it's an authentication error
		if azureErr != nil && (azureErr.StatusCode == 401 || azureErr.StatusCode == 403) {
			l.Warn("Authentication failed when checking storage account. Make sure you have proper permissions")
			return fmt.Errorf("authentication failed when checking storage account: %w", err)
		}
		
		// For other errors, let's assume account is accessible
		l.Infof("Storage account %s appears to be accessible", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
		return nil
	}

	// If we got here without error, the account exists
	l.Infof("Storage account %s exists", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

	// If container exists, clean it up
	if exists {
		err = blobClient.DeleteBlobIfNecessary(ctx, l, "_terragrunt_bootstrap_test", "_terragrunt_bootstrap_test")
		if err != nil {
			l.Warn("Could not clean up test container. This is non-fatal.")
		}
	}

	return nil
}

// GetTFInitArgs returns the subset of config to pass to terraform init.
func (backend *Backend) GetTFInitArgs(backendConfig backend.Config) map[string]any {
	// Azure backend takes all config values and filters out terragruntOnly ones
	return Config(backendConfig).FilterOutTerragruntKeys()
}
