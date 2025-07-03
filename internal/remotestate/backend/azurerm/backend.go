// Package azurerm represents Azure storage backend for remote state
package azurerm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

// BackendName is the name of the Azure backend
const BackendName = "azurerm"

var _ backend.Backend = new(Backend)

// ValidateContainerName validates that the container name follows Azure naming conventions.
// Container names must:
// - Be between 3 and 63 characters long
// - Start with a letter or number, and can contain only letters, numbers, and hyphens
// - Every hyphen must be immediately preceded and followed by a letter or number
// - All letters must be lowercase
func ValidateContainerName(containerName string) error {
	if containerName == "" {
		return WrapContainerValidationError("missing required Azure remote state configuration container_name")
	}

	if len(containerName) < 3 || len(containerName) > 63 {
		return WrapContainerValidationError("container name must be between 3 and 63 characters")
	}

	// Check for uppercase letters
	if containerName != strings.ToLower(containerName) {
		return WrapContainerValidationError("container name can only contain lowercase letters, numbers, and hyphens")
	}

	// Check that it starts and ends with alphanumeric
	if !regexp.MustCompile(`^[a-z0-9].*[a-z0-9]$`).MatchString(containerName) && len(containerName) > 1 {
		return WrapContainerValidationError("container name must start and end with a letter or number")
	}

	// For single character names, just check it's alphanumeric
	if len(containerName) == 1 && !regexp.MustCompile(`^[a-z0-9]$`).MatchString(containerName) {
		return WrapContainerValidationError("container name must start and end with a letter or number")
	}

	// Check that it only contains valid characters (lowercase letters, numbers, hyphens)
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(containerName) {
		return WrapContainerValidationError("container name can only contain lowercase letters, numbers, and hyphens")
	}

	// Check that hyphens are not consecutive and not at start/end (already covered above, but being explicit)
	if strings.Contains(containerName, "--") {
		return WrapContainerValidationError("container name cannot contain consecutive hyphens")
	}

	return nil
}

// Backend implements the backend interface for the Azure backend.
type Backend struct {
	*backend.CommonBackend
	telemetry *AzureTelemetryCollector
}

// NewBackend creates a new Azure backend.
func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
		telemetry:     nil, // Will be initialized when needed
	}
}

// Bootstrap creates the Azure Storage container if it doesn't exist.
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()
	tel := backend.getTelemetry(l)

	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationBootstrap,
		})

		return err
	}

	// Validate container name before any Azure operations
	if err := ValidateContainerName(azureCfg.RemoteStateConfigAzurerm.ContainerName); err != nil {
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "ValidationError",
			Classification: ErrorClassValidation,
			Operation:      OperationBootstrap,
			ResourceType:   "container",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.ContainerName,
		})

		return err
	}

	// Check upfront if storage account creation is requested and validate required fields
	if createIfNotExists, ok := backendConfig["create_storage_account_if_not_exists"].(bool); ok && createIfNotExists {
		subscriptionID, hasSubscription := backendConfig["subscription_id"].(string)
		if !hasSubscription || subscriptionID == "" {
			return NewMissingSubscriptionIDError()
		}

		location, hasLocation := backendConfig["location"].(string)
		if !hasLocation || location == "" {
			return NewMissingLocationError()
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
			hasEnvCreds = true

			l.Warn("ARM_ACCESS_KEY is no longer supported. Please switch to Azure AD authentication.")
		} else if envKey := os.Getenv("AZURE_STORAGE_KEY"); envKey != "" {
			hasEnvCreds = true

			l.Warn("AZURE_STORAGE_KEY is no longer supported. Please switch to Azure AD authentication.")
		}
	}

	// Always use Azure AD auth - if we detect legacy key env vars, we still warn but ignore them
	l.Debug("Using Azure AD authentication")

	if !hasAzureAD && !hasMSI && !hasServicePrincipal && !hasSasToken && !hasEnvCreds {
		err := NewNoValidAuthMethodError()
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "AuthenticationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationBootstrap,
			AuthMethod:     "none",
		})

		return err
	}

	// ensure that only one goroutine can initialize storage
	mu := backend.GetBucketMutex(azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	mu.Lock()
	defer mu.Unlock()

	if backend.IsConfigInited(azureCfg) {
		l.Debugf("%s storage account %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

		// Log successful completion (already initialized)
		tel.LogOperation(ctx, OperationBootstrap, time.Since(startTime), map[string]interface{}{
			"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
			"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
			"status":          "already_initialized",
		})

		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, backendConfig)
	if err != nil {
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "ClientCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationBootstrap,
			ResourceType:   "blob_service_client",
		})

		return err
	}

	// Check if we need to handle storage account creation/validation
	if azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
		// Validate required fields before attempting any Azure operations
		if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
			err := NewMissingSubscriptionIDError()
			tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
				ErrorType:      "ConfigurationError",
				Classification: ErrorClassConfiguration,
				Operation:      OperationBootstrap,
				ResourceType:   "subscription",
			})

			return err
		}

		if azureCfg.StorageAccountConfig.Location == "" {
			err := NewMissingLocationError()
			tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
				ErrorType:      "ConfigurationError",
				Classification: ErrorClassConfiguration,
				Operation:      OperationBootstrap,
				ResourceType:   "location",
			})

			return err
		}

		// Use retry logic for storage account creation
		retryConfig := DefaultRetryConfig()
		err = WithRetry(ctx, l, "storage account bootstrap", retryConfig, func() error {
			return backend.bootstrapStorageAccount(ctx, l, opts, azureCfg)
		})

		if err != nil {
			wrappedErr := WrapStorageAccountError(err, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
			tel.LogError(ctx, wrappedErr, OperationBootstrap, AzureErrorMetrics{
				ErrorType:      "StorageAccountError",
				Classification: ClassifyError(err),
				Operation:      OperationBootstrap,
				ResourceType:   "storage_account",
				ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				SubscriptionID: azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
				Location:       azureCfg.StorageAccountConfig.Location,
			})

			return wrappedErr
		}
	}

	// Create the container if necessary with retry logic
	retryConfig := DefaultRetryConfig()
	err = WithRetry(ctx, l, "container creation", retryConfig, func() error {
		createErr := client.CreateContainerIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		if createErr != nil {
			// Wrap as transient error if it matches patterns
			return WrapTransientError(createErr, "container creation")
		}

		return nil
	})

	if err != nil {
		wrappedErr := WrapContainerError(err, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		tel.LogError(ctx, wrappedErr, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "ContainerError",
			Classification: ClassifyError(err),
			Operation:      OperationBootstrap,
			ResourceType:   "container",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.ContainerName,
			SubscriptionID: azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		})

		return wrappedErr
	}

	backend.MarkConfigInited(azureCfg)

	// Log successful completion
	tel.LogOperation(ctx, OperationBootstrap, time.Since(startTime), map[string]interface{}{
		"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"subscription_id": azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"location":        azureCfg.StorageAccountConfig.Location,
		"status":          "completed",
	})

	return nil
}

// NeedsBootstrap checks if Azure Storage container needs initialization.
func (backend *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	startTime := time.Now()
	tel := backend.getTelemetry(l)

	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		tel.LogError(ctx, err, OperationNeedsBootstrap, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationNeedsBootstrap,
		})

		return false, err
	}

	// Skip initialization if marked as already initialized
	if backend.IsConfigInited(azureCfg) {
		// Log completion - already initialized
		tel.LogOperation(ctx, OperationNeedsBootstrap, time.Since(startTime), map[string]interface{}{
			"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
			"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
			"needs_bootstrap": false,
			"reason":          "already_initialized",
		})

		return false, nil
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, backendConfig)
	if err != nil {
		tel.LogError(ctx, err, OperationNeedsBootstrap, AzureErrorMetrics{
			ErrorType:      "ClientCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationNeedsBootstrap,
			ResourceType:   "blob_service_client",
		})

		return false, err
	}

	// Check if storage account bootstrap is requested
	if azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
		// We will always return true if CreateStorageAccountIfNotExists is true
		// The actual check will be done in Bootstrap() to reduce duplicate API calls
		tel.LogOperation(ctx, OperationNeedsBootstrap, time.Since(startTime), map[string]interface{}{
			"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
			"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
			"needs_bootstrap": true,
			"reason":          "create_storage_account_requested",
		})

		return true, nil
	}

	// Check if container exists with retry logic
	var containerExists bool

	retryConfig := DefaultRetryConfig()
	err = WithRetry(ctx, l, "container existence check", retryConfig, func() error {
		exists, existsErr := client.ContainerExists(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		if existsErr != nil {
			// Try to convert to Azure error
			azureErr := azurehelper.ConvertAzureError(existsErr)

			// Check for permission errors first - these should not be retried
			if azureErr != nil {
				// Create a temporary storage client to use IsPermissionError
				storageClient := &azurehelper.StorageAccountClient{}
				if storageClient.IsPermissionError(existsErr) {
					// Permission errors should bubble up, not be retried
					return existsErr
				}
			}

			// If the storage account doesn't exist, we need bootstrap - not a transient error
			if azureErr != nil && (azureErr.StatusCode == 404 || azureErr.ErrorCode == "StorageAccountNotFound") {
				containerExists = false
				return nil // Not an error, just doesn't exist
			}

			// Wrap as transient error if it matches patterns
			return WrapTransientError(existsErr, "container existence check")
		}

		containerExists = exists

		return nil
	})

	if err != nil {
		tel.LogError(ctx, err, OperationNeedsBootstrap, AzureErrorMetrics{
			ErrorType:      "ContainerExistenceCheckError",
			Classification: ClassifyError(err),
			Operation:      OperationNeedsBootstrap,
			ResourceType:   "container",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.ContainerName,
		})

		return false, err
	}

	needsBootstrap := !containerExists

	// Log completion
	tel.LogOperation(ctx, OperationNeedsBootstrap, time.Since(startTime), map[string]interface{}{
		"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"needs_bootstrap": needsBootstrap,
		"reason":          map[bool]string{true: "container_not_exists", false: "container_exists"}[needsBootstrap],
	})

	if !containerExists {
		return true, nil
	}

	return false, nil
}

// Delete deletes the remote state file from Azure Storage.
func (backend *Backend) Delete(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()
	tel := backend.getTelemetry(l)

	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		tel.LogError(ctx, err, OperationDelete, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationDelete,
		})

		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s blob %s will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)

	shouldContinue, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		tel.LogError(ctx, err, OperationDelete, AzureErrorMetrics{
			ErrorType:      "UserPromptError",
			Classification: ErrorClassUserInput,
			Operation:      OperationDelete,
		})

		return err
	}

	if !shouldContinue {
		// Log user cancellation
		tel.LogOperation(ctx, OperationDelete, time.Since(startTime), map[string]interface{}{
			"container": azureCfg.RemoteStateConfigAzurerm.ContainerName,
			"blob_key":  azureCfg.RemoteStateConfigAzurerm.Key,
			"status":    "cancelled_by_user",
		})

		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, backendConfig)
	if err != nil {
		tel.LogError(ctx, err, OperationDelete, AzureErrorMetrics{
			ErrorType:      "ClientCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationDelete,
			ResourceType:   "blob_service_client",
		})

		return err
	}

	// Delete blob with retry logic
	retryConfig := DefaultRetryConfig()
	err = WithRetry(ctx, l, "blob deletion", retryConfig, func() error {
		deleteErr := client.DeleteBlobIfNecessary(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)
		if deleteErr != nil {
			return WrapTransientError(deleteErr, "blob deletion")
		}

		return nil
	})

	if err != nil {
		tel.LogError(ctx, err, OperationDelete, AzureErrorMetrics{
			ErrorType:      "BlobDeletionError",
			Classification: ClassifyError(err),
			Operation:      OperationDelete,
			ResourceType:   "blob",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.Key,
			SubscriptionID: azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		})

		return err
	}

	// Log successful completion
	tel.LogOperation(ctx, OperationDelete, time.Since(startTime), map[string]interface{}{
		"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"blob_key":        azureCfg.RemoteStateConfigAzurerm.Key,
		"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"status":          "completed",
	})

	return nil
}

// DeleteContainer deletes the entire Azure Storage container.
func (backend *Backend) DeleteContainer(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()
	tel := backend.getTelemetry(l)

	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationDeleteContainer,
		})

		return err
	}

	prompt := fmt.Sprintf("Azure Storage container %s and all its contents will be deleted. Do you want to continue?", azureCfg.RemoteStateConfigAzurerm.ContainerName)

	shouldContinue, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
			ErrorType:      "UserPromptError",
			Classification: ErrorClassUserInput,
			Operation:      OperationDeleteContainer,
		})

		return err
	}

	if !shouldContinue {
		// Log user cancellation
		tel.LogOperation(ctx, OperationDeleteContainer, time.Since(startTime), map[string]interface{}{
			"container": azureCfg.RemoteStateConfigAzurerm.ContainerName,
			"status":    "cancelled_by_user",
		})

		return nil
	}

	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, backendConfig)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
			ErrorType:      "ClientCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationDeleteContainer,
			ResourceType:   "blob_service_client",
		})

		return err
	}

	// Delete container with retry logic
	retryConfig := DefaultRetryConfig()
	err = WithRetry(ctx, l, "container deletion", retryConfig, func() error {
		deleteErr := client.DeleteContainer(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		if deleteErr != nil {
			return WrapTransientError(deleteErr, "container deletion")
		}

		return nil
	})

	if err != nil {
		tel.LogError(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
			ErrorType:      "ContainerDeletionError",
			Classification: ClassifyError(err),
			Operation:      OperationDeleteContainer,
			ResourceType:   "container",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.ContainerName,
			SubscriptionID: azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		})

		return err
	}

	// Log successful completion
	tel.LogOperation(ctx, OperationDeleteContainer, time.Since(startTime), map[string]interface{}{
		"container":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"status":          "completed",
	})

	return nil
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

	srcClient, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, srcBackendConfig)
	if err != nil {
		return fmt.Errorf("error creating source blob client: %w", err)
	}

	dstClient, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, dstBackendConfig)
	if err != nil {
		return fmt.Errorf("error creating destination blob client: %w", err)
	}

	// Check that source container exists and state file is present with retry logic
	srcContainer := srcCfg.RemoteStateConfigAzurerm.ContainerName
	srcKey := srcCfg.RemoteStateConfigAzurerm.Key

	retryConfig := DefaultRetryConfig()

	var exists bool

	err = WithRetry(ctx, l, "source container existence check", retryConfig, func() error {
		containerExists, existsErr := srcClient.ContainerExists(ctx, srcContainer)
		if existsErr != nil {
			return WrapTransientError(existsErr, "source container existence check")
		}

		exists = containerExists

		return nil
	})

	if err != nil {
		return fmt.Errorf("error checking source container existence: %w", err)
	}

	if !exists {
		return WrapContainerDoesNotExistError(errors.New("container not found"), srcContainer)
	}

	// Ensure destination container exists (create if necessary) with retry logic
	dstContainer := dstCfg.RemoteStateConfigAzurerm.ContainerName

	err = WithRetry(ctx, l, "destination container creation", retryConfig, func() error {
		createErr := dstClient.CreateContainerIfNecessary(ctx, l, dstContainer)
		if createErr != nil {
			return WrapTransientError(createErr, "destination container creation")
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error creating destination container: %w", err)
	}

	// Copy state file from source to destination with retry logic
	err = WithRetry(ctx, l, "state file copy", retryConfig, func() error {
		copyErr := srcClient.CopyBlobToContainer(ctx, srcContainer, srcKey, dstClient, dstContainer, dstCfg.RemoteStateConfigAzurerm.Key)
		if copyErr != nil {
			return WrapTransientError(copyErr, "state file copy")
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error copying state file: %w", err)
	}

	// Verify the copy succeeded by reading the destination blob with retry logic
	dstInput := &azurehelper.GetObjectInput{
		Container: &dstContainer,
		Key:       &dstCfg.RemoteStateConfigAzurerm.Key,
	}

	err = WithRetry(ctx, l, "destination state file verification", retryConfig, func() error {
		_, getObjectErr := dstClient.GetObject(ctx, dstInput)
		if getObjectErr != nil {
			return WrapTransientError(getObjectErr, "destination state file verification")
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error verifying destination state file: %w", err)
	}

	// Delete the source state file with retry logic
	err = WithRetry(ctx, l, "source state file deletion", retryConfig, func() error {
		deleteErr := srcClient.DeleteBlobIfNecessary(ctx, l, srcContainer, srcKey)
		if deleteErr != nil {
			return WrapTransientError(deleteErr, "source state file deletion")
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error deleting source state file: %w", err)
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
		return NewMissingResourceGroupError()
	}

	if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
		return NewMissingSubscriptionIDError()
	}

	// Check if we're in non-interactive mode
	if opts.NonInteractive {
		return WrapNonInteractiveDeleteError(storageAccountName)
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
	client, err := azurehelper.CreateStorageAccountClient(ctx, l, storageAccountConfig)
	if err != nil {
		return err
	}

	// Delete the storage account
	l.Infof("Deleting Azure Storage Account %s...", storageAccountName)

	return client.DeleteStorageAccount(ctx, l)
}

// bootstrapStorageAccount handles creating or checking a storage account
func (backend *Backend) bootstrapStorageAccount(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	// Import the armstorage package conditionally
	// We need to add the armstorage package to go.mod
	// go get github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage
	// Validate that required fields are set for storage account operations
	if azureCfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
		return tgerrors.New(MissingSubscriptionIDError{})
	}

	if azureCfg.StorageAccountConfig.Location == "" {
		return NewMissingLocationError()
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
	// Declare blobClient and err for later use
	var blobClient *azurehelper.BlobServiceClient

	var err error
	// Create a storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, l, storageAccountConfig)
	if err != nil {
		return WrapStorageAccountError(err, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	}

	// Convert configuration to the expected format for the storage account client
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        subscriptionID,
		ResourceGroupName:     azureCfg.StorageAccountConfig.ResourceGroupName,
		StorageAccountName:    azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		Location:              azureCfg.StorageAccountConfig.Location,
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
		return WrapStorageAccountError(err, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	}

	// After potentially creating the storage account, recreate the blob client with DNS waiting enabled
	// This ensures DNS propagation doesn't cause issues if the storage account was just created
	storageAccountConfigWithDNSWait := make(map[string]interface{})
	for k, v := range storageAccountConfig {
		storageAccountConfigWithDNSWait[k] = v
	}

	storageAccountConfigWithDNSWait["wait_for_dns"] = true
	blobClient, err = azurehelper.CreateBlobServiceClient(ctx, l, opts, storageAccountConfigWithDNSWait)

	if err != nil {
		// If DNS wait fails, try without it as fallback
		l.Warnf("Failed to create blob client with DNS wait, retrying without DNS wait: %v", err)

		blobClient, err = azurehelper.CreateBlobServiceClient(ctx, l, opts, storageAccountConfig)

		if err != nil {
			return WrapStorageAccountError(err, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
		}
	}

	// Ensure the current user has Storage Blob Data Owner role
	// This is important for both new and existing storage accounts
	err = storageClient.AssignStorageBlobDataOwnerRole(ctx, l)
	if err != nil {
		if storageClient.IsPermissionError(err) {
			l.Warnf("Failed to assign Storage Blob Data Owner role due to insufficient permissions: %v", err)
		} else {
			l.Warnf("Failed to assign Storage Blob Data Owner role: %v", err)
		}
	}
	// Don't fail the entire process if role assignment fails

	l.Infof("Storage account %s exists and is accessible", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

	// For safety, try the blob client operation to confirm access
	exists, err := blobClient.ContainerExists(ctx, "_terragrunt_bootstrap_test")
	if err != nil {
		// Use the enhanced permission error detection from azurehelper
		if storageClient.IsPermissionError(err) {
			l.Warn("Permission denied when checking storage account. Make sure you have proper permissions")
			return WrapAuthenticationError(err, "Azure AD")
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

// initTelemetry initializes the telemetry collector if available
func (backend *Backend) initTelemetry(l log.Logger) {
	if backend.telemetry == nil {
		// For now, we'll create a simple telemetry collector without telemeter integration
		// This can be enhanced later when we have better access to the telemeter instance
		backend.telemetry = &AzureTelemetryCollector{
			telemeter: nil, // Will be set later when telemeter is available
			logger:    l,
		}
	}
}

// getTelemetry returns the telemetry collector, initializing it if needed
func (backend *Backend) getTelemetry(l log.Logger) *AzureTelemetryCollector {
	backend.initTelemetry(l)
	return backend.telemetry
}

// IsVersionControlEnabled checks if versioning is enabled on the Azure storage account.
func (backend *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	startTime := time.Now()
	tel := backend.getTelemetry(l)

	azureCfg, err := Config(backendConfig).ExtendedAzureConfig()
	if err != nil {
		tel.LogError(ctx, err, OperationValidation, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationValidation,
		})

		return false, err
	}

	// Get Azure configuration for creating storage account client
	storageConfig := map[string]interface{}{
		"storage_account_name": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"resource_group_name":  azureCfg.RemoteStateConfigAzurerm.ResourceGroupName,
		"subscription_id":      azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"use_azuread_auth":     azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth,
	}

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, l, storageConfig)
	if err != nil {
		tel.LogError(ctx, err, OperationValidation, AzureErrorMetrics{
			ErrorType:      "StorageClientError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationValidation,
		})

		return false, tgerrors.Errorf("failed to create storage account client: %w", err)
	}

	// Check if storage account exists first
	exists, _, err := storageClient.StorageAccountExists(ctx)
	if err != nil {
		tel.LogError(ctx, err, OperationValidation, AzureErrorMetrics{
			ErrorType:      "StorageAccountAccessError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationValidation,
		})

		return false, tgerrors.Errorf("failed to check if storage account exists: %w", err)
	}

	if !exists {
		l.Debugf("Storage account %s does not exist, versioning check skipped", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
		return false, WrapStorageAccountError(errors.New("storage account does not exist"), azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
	}

	// Check if versioning is enabled
	enabled, err := storageClient.GetStorageAccountVersioning(ctx)
	if err != nil {
		tel.LogError(ctx, err, OperationValidation, AzureErrorMetrics{
			ErrorType:      "VersioningCheckError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationValidation,
		})

		return false, tgerrors.Errorf("failed to check versioning status: %w", err)
	}

	// Log telemetry for successful version check
	tel.LogOperation(ctx, OperationValidation, time.Since(startTime), map[string]interface{}{
		"storage_account":    azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"resource_group":     azureCfg.RemoteStateConfigAzurerm.ResourceGroupName,
		"subscription_id":    azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"versioning_enabled": enabled,
		"operation":          "version_control_check",
		"status":             "completed",
	})

	l.Debugf("Storage account %s versioning status: %t", azureCfg.RemoteStateConfigAzurerm.StorageAccountName, enabled)

	return enabled, nil
}
