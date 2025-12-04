// Package azurerm represents Azure storage backend for remote state
package azurerm

import (
	"bytes"
	"context"

	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureutil"

	azErrors "github.com/gruntwork-io/terragrunt/internal/azure/errors"
	"github.com/gruntwork-io/terragrunt/internal/azure/factory"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

// BackendName is the name of the Azure backend
const BackendName = "azurerm"

const (
	defaultRetryDelaySeconds    = 1
	defaultMaxDelaySeconds      = 30
	defaultMetricsBufferSize    = 1000
	defaultFlushIntervalSeconds = 30
	defaultCacheTimeoutSeconds  = 300
	defaultMaxCacheSize         = 100
	defaultAuthCacheTimeoutSecs = 3600
	defaultRetryMaxAttempts     = 3
)

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
	telemetry        *AzureTelemetryCollector
	errorHandler     *azureutil.ErrorHandler
	serviceContainer interfaces.AzureServiceContainer
	serviceFactory   interfaces.ServiceFactory
}

// BackendConfig holds configuration for the backend
type BackendConfig struct {
	// ServiceFactory creates Azure services
	ServiceFactory interfaces.ServiceFactory
	// Telemetry collector for Azure operations
	Telemetry *AzureTelemetryCollector
	// ErrorHandler for Azure operations
	ErrorHandler *azureutil.ErrorHandler
	// RetryConfig contains retry configuration settings
	RetryConfig *interfaces.RetryConfig
	// TelemetrySettings configures telemetry collection behavior
	TelemetrySettings *TelemetrySettings
	// CacheSettings configures service caching behavior
	CacheSettings *CacheSettings
	// AuthSettings configures authentication behavior
	AuthSettings *AuthSettings
}

// TelemetrySettings configures telemetry collection behavior
type TelemetrySettings struct {
	// MetricsBufferSize sets the buffer size for metrics collection
	MetricsBufferSize int
	// FlushInterval sets how often metrics are flushed (in seconds)
	FlushInterval int
	// EnableDetailedMetrics enables collection of detailed performance metrics
	EnableDetailedMetrics bool
	// EnableErrorTracking enables detailed error tracking and classification
	EnableErrorTracking bool
}

// CacheSettings configures service caching behavior
type CacheSettings struct {
	// CacheTimeout sets cache timeout in seconds
	CacheTimeout int
	// MaxCacheSize sets maximum number of cached instances
	MaxCacheSize int
	// EnableCaching enables service instance caching
	EnableCaching bool
	// EnableCacheMetrics enables cache performance metrics
	EnableCacheMetrics bool
}

// AuthSettings configures authentication behavior
type AuthSettings struct {
	// PreferredAuthMethod sets the preferred authentication method
	PreferredAuthMethod string
	// AuthCacheTimeout sets auth cache timeout in seconds
	AuthCacheTimeout int
	// EnableAuthCaching enables authentication token caching
	EnableAuthCaching bool
	// EnableAuthRetry enables automatic auth retry on token expiration
	EnableAuthRetry bool
}

// NewBackend creates a new Azure backend with the given configuration.
func NewBackend(cfg *BackendConfig) *Backend {
	return NewBackendWithContext(context.Background(), cfg)
}

// NewBackendWithContext creates a backend using the provided context.
func NewBackendWithContext(ctx context.Context, cfg *BackendConfig) *Backend {
	if cfg == nil {
		cfg = &BackendConfig{}
	}

	// Apply default settings if not provided
	applyDefaultBackendConfig(cfg)

	// Configure factory with enhanced options if none provided
	if cfg.ServiceFactory == nil {
		// Create factory with enhanced configuration based on backend config
		factoryOptions := createFactoryOptionsFromConfig(cfg)

		// Create enhanced factory with options
		factory := factory.NewAzureServiceFactoryWithOptions(factoryOptions)
		cfg.ServiceFactory = &enhancedServiceFactory{
			container:         factory,
			telemetrySettings: cfg.TelemetrySettings,
			authSettings:      cfg.AuthSettings,
		}
	}

	// Create new service container with context
	serviceContainer := cfg.ServiceFactory.CreateContainer(ctx)

	// Initialize telemetry with enhanced settings if not provided
	if cfg.Telemetry == nil && cfg.TelemetrySettings != nil && cfg.TelemetrySettings.EnableDetailedMetrics {
		// We'll initialize this later when we have a logger with the enhanced settings
		cfg.Telemetry = nil
	}

	// Initialize error handler if not provided
	if cfg.ErrorHandler == nil {
		// We'll initialize this later when we have telemetry and logger
		cfg.ErrorHandler = nil
	}

	return &Backend{
		CommonBackend:    backend.NewCommonBackend(BackendName),
		telemetry:        cfg.Telemetry,
		errorHandler:     cfg.ErrorHandler,
		serviceContainer: serviceContainer,
		serviceFactory:   cfg.ServiceFactory,
	}
}

// NewBackendFromRemoteStateConfig creates a backend from remote state configuration
// This method allows creation of a backend with configuration extracted from backend.Config
func NewBackendFromRemoteStateConfig(remoteStateConfig backend.Config, opts *options.TerragruntOptions) (*Backend, error) {
	cfg := &BackendConfig{}

	// Extract configuration from remote state config
	extractBackendConfigFromRemoteState(remoteStateConfig, cfg, opts)

	return NewBackend(cfg), nil
}

// extractBackendConfigFromRemoteState extracts backend configuration from remote state config
func extractBackendConfigFromRemoteState(remoteStateConfig backend.Config, cfg *BackendConfig, opts *options.TerragruntOptions) {
	// Set retry configuration based on defaults
	cfg.RetryConfig = &interfaces.RetryConfig{
		MaxRetries: options.DefaultRetryMaxAttempts,
		RetryDelay: defaultRetryDelaySeconds,
		MaxDelay:   defaultMaxDelaySeconds,
		RetryableStatusCodes: []int{
			http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		},
	}

	// Configure telemetry settings based on options
	cfg.TelemetrySettings = &TelemetrySettings{
		EnableDetailedMetrics: true, // Default to enabled
		EnableErrorTracking:   true,
		MetricsBufferSize:     defaultMetricsBufferSize,
		FlushInterval:         defaultFlushIntervalSeconds,
	}

	// Configure cache settings
	cfg.CacheSettings = &CacheSettings{
		EnableCaching:      true,
		CacheTimeout:       defaultCacheTimeoutSeconds,
		MaxCacheSize:       defaultMaxCacheSize,
		EnableCacheMetrics: false,
	}

	// Configure auth settings based on remote state config
	cfg.AuthSettings = &AuthSettings{
		PreferredAuthMethod: extractPreferredAuthMethod(remoteStateConfig),
		EnableAuthCaching:   true,
		AuthCacheTimeout:    defaultAuthCacheTimeoutSecs,
		EnableAuthRetry:     true,
	}
}

// extractPreferredAuthMethod determines the preferred authentication method from config
func extractPreferredAuthMethod(config backend.Config) string {
	// Check for various auth methods in order of preference
	if useAzureAD, ok := config["use_azuread_auth"].(bool); ok && useAzureAD {
		return "azuread"
	}

	if useMSI, ok := config["use_msi"].(bool); ok && useMSI {
		return "msi"
	}

	if clientID, ok := config["client_id"].(string); ok && clientID != "" {
		return "service_principal"
	}

	if sasToken, ok := config["sas_token"].(string); ok && sasToken != "" {
		return "sas_token"
	}

	if accessKey, ok := config["access_key"].(string); ok && accessKey != "" {
		return "access_key"
	}

	// Default to Azure AD
	return "azuread"
}

// GetTFInitArgs returns the config that should be passed on to `tofu -backend-config` cmd line param
func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).FilterOutTerragruntKeys()
}

// Bootstrap creates the Azure Storage container if it doesn't exist.
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()
	tel := backend.getTelemetry(l)
	// Also get the error handler - this will initialize telemetry as well
	errorHandler := backend.getErrorHandler(l)

	// Parse and validate the Azure config
	var azureCfg *ExtendedRemoteStateConfigAzurerm

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationBootstrap,
		"config",
		"azurerm",
		func() error {
			var configErr error

			azureCfg, configErr = Config(backendConfig).ExtendedAzureConfig()

			return configErr
		},
	)
	if err != nil {
		return err
	}

	// Validate container name before any Azure operations
	containerName := azureCfg.RemoteStateConfigAzurerm.ContainerName

	err = errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationValidation,
		"container",
		containerName,
		func() error {
			return ValidateContainerName(containerName)
		},
	)
	if err != nil {
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

	// Use the new centralized authentication package to handle auth configuration
	authConfig, err := azureauth.GetAuthConfig(ctx, l, backendConfig)
	if err != nil {
		return err
	}
	// Validate the authentication configuration
	if err := azureauth.ValidateAuthConfig(authConfig); err != nil {
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "AuthValidationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationBootstrap,
			AuthMethod:     string(authConfig.Method),
		})

		return err
	}

	// Update the Azure config with the normalized authentication values
	// This ensures consistency between our new auth package and the existing config
	if authConfig.UseAzureAD && !azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth {
		azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth = true
		backendConfig["use_azuread_auth"] = true

		l.Info("Azure AD authentication is now the default and required authentication method")
	}

	// If using the new auth package detected credentials from environment variables,
	// update the config to reflect this
	if authConfig.UseEnvironment {
		azureCfg.RemoteStateConfigAzurerm.SubscriptionID = authConfig.SubscriptionID
		azureCfg.RemoteStateConfigAzurerm.TenantID = authConfig.TenantID
		azureCfg.RemoteStateConfigAzurerm.ClientID = authConfig.ClientID
		azureCfg.RemoteStateConfigAzurerm.ClientSecret = authConfig.ClientSecret
		azureCfg.RemoteStateConfigAzurerm.SasToken = authConfig.SasToken

		// Update the backend config as well for consistency
		if authConfig.SubscriptionID != "" {
			backendConfig["subscription_id"] = authConfig.SubscriptionID
		}
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

	// Check if we need to handle storage account creation first
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
			return backend.bootstrapStorageAccount(ctx, l, azureCfg)
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

	// Get the blob service using our helper - do this after storage account is created
	client, err := backend.getBlobService(ctx, l, azureCfg, opts)
	if err != nil {
		tel.LogError(ctx, err, OperationBootstrap, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationBootstrap,
			ResourceType:   "blob_service",
		})

		return err
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

	_, err = backend.getBlobService(ctx, l, azureCfg, opts)
	if err != nil {
		tel.LogError(ctx, err, OperationNeedsBootstrap, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassAuthentication,
			Operation:      OperationNeedsBootstrap,
			ResourceType:   "blob_service",
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
		// Get the blob service from the container
		blobService, err := backend.getBlobService(ctx, l, azureCfg, opts)
		if err != nil {
			return fmt.Errorf("failed to get blob service: %w", err)
		}

		exists, existsErr := blobService.ContainerExists(ctx, azureCfg.RemoteStateConfigAzurerm.ContainerName)
		if existsErr != nil {
			// Try to convert to Azure error
			azureErr := azurehelper.ConvertAzureError(existsErr)

			// Check for permission errors first - these should not be retried
			if azureErr != nil {
				if azureutil.IsPermissionError(existsErr) {
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

	backend.initTelemetry(l)

	tel := backend.telemetry
	errorHandler := backend.getErrorHandler(l)

	// Parse the Azure configuration with error handling
	var azureCfg *ExtendedRemoteStateConfigAzurerm

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationDelete,
		"config",
		"azurerm",
		func() error {
			var configErr error

			azureCfg, configErr = Config(backendConfig).ExtendedAzureConfig()

			return configErr
		},
	)
	if err != nil {
		return err
	}

	// Prompt user for confirmation
	shouldContinue, err := backend.promptForBlobDeletion(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key, opts)
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

	// Get blob service from container
	blobService, err := backend.GetBlobServiceFromConfig(ctx, l, azureCfg)
	if err != nil {
		tel.LogError(ctx, err, OperationDelete, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationDelete,
			ResourceType:   "blob_service",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		})

		return err
	}

	// Delete the blob
	err = backend.deleteBlobWithRetry(ctx, l, blobService, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key)
	if err != nil {
		tel.LogErrorWithMetrics(ctx, err, OperationDelete, AzureErrorMetrics{
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
	backend.logDeleteBlobSuccess(ctx, tel, startTime, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.Key, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

	return nil
}

// DeleteContainer deletes the entire Azure Storage container.
func (backend *Backend) DeleteContainer(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()

	backend.initTelemetry(l)

	tel := backend.telemetry
	errorHandler := backend.getErrorHandler(l)

	// Parse the Azure configuration with error handling
	var azureCfg *ExtendedRemoteStateConfigAzurerm

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationDeleteContainer,
		"config",
		"azurerm",
		func() error {
			var configErr error

			azureCfg, configErr = Config(backendConfig).ExtendedAzureConfig()

			return configErr
		},
	)
	if err != nil {
		return err
	}

	// Prompt user for confirmation
	shouldContinue, err := backend.promptForContainerDeletion(ctx, l, azureCfg.RemoteStateConfigAzurerm.ContainerName, opts)
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

	// Get blob service from container
	blobService, err := backend.GetBlobServiceFromConfig(ctx, l, azureCfg)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationDeleteContainer,
			ResourceType:   "blob_service",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		})

		return err
	}

	err = backend.deleteContainerWithRetry(ctx, l, blobService, azureCfg.RemoteStateConfigAzurerm.ContainerName)
	if err != nil {
		tel.LogErrorWithMetrics(ctx, err, OperationDeleteContainer, AzureErrorMetrics{
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
	backend.logDeleteContainerSuccess(ctx, tel, startTime, azureCfg.RemoteStateConfigAzurerm.ContainerName, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

	return nil
}

// promptForContainerDeletion prompts the user to confirm deletion of a container.
func (backend *Backend) promptForContainerDeletion(ctx context.Context, l log.Logger, containerName string, opts *options.TerragruntOptions) (bool, error) {
	prompt := fmt.Sprintf("Azure Storage container %s and all its contents will be deleted. Do you want to continue?", containerName)
	return shell.PromptUserForYesNo(ctx, l, prompt, opts)
}

// promptForBlobDeletion prompts the user to confirm deletion of a blob.
func (backend *Backend) promptForBlobDeletion(ctx context.Context, l log.Logger, containerName, blobKey string, opts *options.TerragruntOptions) (bool, error) {
	prompt := fmt.Sprintf("Azure Storage container %s blob %s will be deleted. Do you want to continue?", containerName, blobKey)
	return shell.PromptUserForYesNo(ctx, l, prompt, opts)
}

// deleteContainerWithRetry deletes a container with retry logic.
func (backend *Backend) deleteContainerWithRetry(ctx context.Context, l log.Logger, blobService interfaces.BlobService, containerName string) error {
	retryConfig := DefaultRetryConfig()

	return WithRetry(ctx, l, "container deletion", retryConfig, func() error {
		deleteErr := blobService.DeleteContainer(ctx, l, containerName)
		if deleteErr != nil {
			return WrapTransientError(deleteErr, "container deletion")
		}

		return nil
	})
}

// logDeleteContainerSuccess logs successful container deletion telemetry.
func (backend *Backend) logDeleteContainerSuccess(ctx context.Context, tel *AzureTelemetryCollector, startTime time.Time, containerName, storageAccountName string) {
	tel.LogOperation(ctx, OperationDeleteContainer, time.Since(startTime), map[string]interface{}{
		"container":       containerName,
		"storage_account": storageAccountName,
		"status":          "completed",
	})
}

// deleteBlobWithRetry deletes a blob with retry logic to handle transient errors.
func (backend *Backend) deleteBlobWithRetry(ctx context.Context, l log.Logger, blobService interfaces.BlobService, containerName, blobKey string) error {
	retryConfig := DefaultRetryConfig()

	return WithRetry(ctx, l, "blob deletion", retryConfig, func() error {
		deleteErr := blobService.DeleteBlobIfNecessary(ctx, l, containerName, blobKey)
		if deleteErr != nil {
			return WrapTransientError(deleteErr, "blob deletion")
		}

		return nil
	})
}

// logDeleteBlobSuccess logs successful blob deletion telemetry.
func (backend *Backend) logDeleteBlobSuccess(ctx context.Context, tel *AzureTelemetryCollector, startTime time.Time, containerName, blobKey, storageAccountName string) {
	tel.LogOperation(ctx, OperationDelete, time.Since(startTime), map[string]interface{}{
		"container":       containerName,
		"blob_key":        blobKey,
		"storage_account": storageAccountName,
		"status":          "completed",
	})
}

// Convert io.ReadCloser to []byte and handle upload with retries
func (backend *Backend) uploadBlobFromReader(
	ctx context.Context,
	l log.Logger,
	blobService interfaces.BlobService,
	containerName,
	blobName string,
	data io.ReadCloser,
) error {
	// Define a reasonable max size for state files (500MB)
	const maxStateFileSize = 500 * 1024 * 1024

	// If the reader provides size info, check it
	if sizer, ok := data.(interface{ Size() int64 }); ok {
		if size := sizer.Size(); size > maxStateFileSize {
			return fmt.Errorf("state file too large: %d bytes exceeds limit of %d bytes", size, maxStateFileSize)
		}
	}

	// Enforce max size even if Size() is not available using LimitReader
	// This prevents reading unbounded data into memory
	limitedReader := io.LimitReader(data, maxStateFileSize+1)

	// Read all data from the limited reader
	blobData, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("error reading blob data: %w", err)
	}

	// Check if we hit the limit (read more than maxStateFileSize)
	if int64(len(blobData)) > maxStateFileSize {
		return fmt.Errorf("state file too large: exceeds limit of %d bytes", maxStateFileSize)
	}

	defer func() {
		_ = data.Close()
	}()

	// Upload the blob with retry logic
	retryConfig := DefaultRetryConfig()

	return WithRetry(ctx, l, "blob upload", retryConfig, func() error {
		uploadErr := blobService.UploadBlob(ctx, l, containerName, blobName, blobData)
		if uploadErr != nil {
			return WrapTransientError(uploadErr, "blob upload")
		}

		return nil
	})
}

// WrapBlobError wraps an error with blob-specific context
func (backend *Backend) wrapBlobError(err error, container, key string) error {
	return azErrors.WrapBlobError(err, container, key)
}

// WrapStorageAccountError wraps an error with storage account context
func (backend *Backend) wrapStorageAccountError(err error, accountName string) error {
	return azErrors.WrapStorageAccountError(err, accountName)
}

// WrapContainerError wraps an error with container context
func (backend *Backend) wrapContainerError(err error, containerName string) error {
	return azErrors.WrapContainerError(err, containerName)
}

// WrapContainerDoesNotExistError wraps an error indicating a container does not exist
func (backend *Backend) wrapContainerDoesNotExistError(err error, containerName string) error {
	return azErrors.WrapContainerDoesNotExistError(err, containerName)
}

// Migrate copies the state file from source container to destination container and deletes the original.
func (backend *Backend) Migrate(ctx context.Context, l log.Logger, srcBackendConfig, dstBackendConfig backend.Config, opts *options.TerragruntOptions) error {
	// If not using force flag, warn about versioning being a storage account level setting
	if !opts.ForceBackendMigrate {
		l.Warn("Warning: Blob versioning in Azure Storage is a storage account level setting. Use the Azure Portal or CLI to verify that blob versioning is enabled on both source and destination storage accounts.")
	}

	// Get error handler
	errorHandler := backend.getErrorHandler(l)

	// Parse source and destination configurations
	srcCfg, dstCfg, err := backend.parseMigrateConfigs(ctx, errorHandler, srcBackendConfig, dstBackendConfig)
	if err != nil {
		return err
	}

	// Get source and destination blob services
	srcBlobService, err := backend.getBlobService(ctx, l, srcCfg, opts)
	if err != nil {
		return err
	}

	dstBlobService, err := backend.getBlobService(ctx, l, dstCfg, opts)
	if err != nil {
		return err
	}

	// Verify source container and prepare destination container
	srcContainer := srcCfg.RemoteStateConfigAzurerm.ContainerName
	srcKey := srcCfg.RemoteStateConfigAzurerm.Key
	dstContainer := dstCfg.RemoteStateConfigAzurerm.ContainerName
	dstKey := dstCfg.RemoteStateConfigAzurerm.Key

	// Validate source container exists
	exists, err := srcBlobService.ContainerExists(ctx, srcContainer)
	if err != nil {
		return backend.wrapContainerError(err, srcContainer)
	}

	if !exists {
		return backend.wrapContainerDoesNotExistError(errors.New("container not found"), srcContainer)
	}

	// Create destination container if needed
	if err := dstBlobService.CreateContainerIfNecessary(ctx, l, dstContainer); err != nil {
		return backend.wrapContainerError(err, dstContainer)
	}

	// Get source object using the interface-based method
	srcOutput, err := backend.GetObject(ctx, l, srcContainer, srcKey, srcBlobService)
	if err != nil {
		return err
	}

	// Upload blob to destination
	srcReader := io.NopCloser(bytes.NewReader(srcOutput))
	if err := backend.uploadBlobFromReader(ctx, l, dstBlobService, dstContainer, dstKey, srcReader); err != nil {
		return backend.wrapBlobError(err, dstContainer, dstKey)
	}

	// Verify the copy succeeded by reading the destination blob
	if _, err := backend.GetObject(ctx, l, dstContainer, dstKey, dstBlobService); err != nil {
		return fmt.Errorf("error verifying destination state file: %w", err)
	}

	// Delete source state file
	if err := srcBlobService.DeleteBlobIfNecessary(ctx, l, srcContainer, srcKey); err != nil {
		return fmt.Errorf("error deleting source state file: %w", err)
	}

	return nil
}

// parseMigrateConfigs parses the source and destination Azure configurations.
func (backend *Backend) parseMigrateConfigs(
	ctx context.Context,
	errorHandler *azureutil.ErrorHandler,
	srcBackendConfig, dstBackendConfig backend.Config,
) (*ExtendedRemoteStateConfigAzurerm, *ExtendedRemoteStateConfigAzurerm, error) {
	var (
		srcCfg, dstCfg *ExtendedRemoteStateConfigAzurerm
		srcErr, dstErr error
	)

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationStorageOp,
		"config",
		"source",
		func() error {
			srcCfg, srcErr = Config(srcBackendConfig).ExtendedAzureConfig()
			return srcErr
		},
	)
	if err != nil {
		return nil, nil, err
	}

	err = errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationStorageOp,
		"config",
		"destination",
		func() error {
			dstCfg, dstErr = Config(dstBackendConfig).ExtendedAzureConfig()
			return dstErr
		},
	)
	if err != nil {
		return nil, nil, err
	}

	return srcCfg, dstCfg, nil
}

// DeleteStorageAccount deletes an Azure Storage account.
func (backend *Backend) DeleteStorageAccount(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	startTime := time.Now()
	tel := backend.getTelemetry(l)
	errorHandler := backend.getErrorHandler(l)

	// Parse and validate the Azure config with error handling
	var azureCfg *ExtendedRemoteStateConfigAzurerm

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationDeleteAccount,
		"config",
		"azurerm",
		func() error {
			var configErr error

			azureCfg, configErr = Config(backendConfig).ExtendedAzureConfig()

			return configErr
		},
	)
	if err != nil {
		return err
	}

	// Extract key values and validate
	var (
		storageAccountName = azureCfg.RemoteStateConfigAzurerm.StorageAccountName
		resourceGroupName  = azureCfg.StorageAccountConfig.ResourceGroupName
		subscriptionID     = azureCfg.RemoteStateConfigAzurerm.SubscriptionID
	)

	// Validate required fields
	if err := backend.validateDeleteStorageAccountConfig(resourceGroupName, subscriptionID); err != nil {
		tel.LogError(ctx, err, OperationDeleteAccount, AzureErrorMetrics{
			ErrorType:      "ConfigValidationError",
			Classification: ErrorClassConfiguration,
			Operation:      "delete_account",
		})

		return err
	}

	// Check if we're in non-interactive mode
	if opts.NonInteractive {
		return WrapNonInteractiveDeleteError(storageAccountName)
	}

	// Ask for confirmation
	shouldContinue, err := backend.promptForStorageAccountDeletion(ctx, l, storageAccountName, opts)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteAccount, AzureErrorMetrics{
			ErrorType:      "UserPromptError",
			Classification: ErrorClassUserInput,
			Operation:      "delete_account",
		})

		return err
	}

	if !shouldContinue {
		// Log user cancellation
		tel.LogOperation(ctx, OperationDeleteAccount, time.Since(startTime), map[string]interface{}{
			"storage_account": storageAccountName,
			"status":          "cancelled_by_user",
		})

		return nil
	}

	// Get the storage account service using our helper
	storageService, err := backend.getStorageAccountService(ctx, l, azureCfg)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteAccount, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationDeleteAccount,
			ResourceType:   "storage_account",
			ResourceName:   storageAccountName,
			SubscriptionID: subscriptionID,
		})

		return err
	}

	// Delete the storage account
	err = backend.deleteStorageAccountWithRetry(ctx, l, storageService, resourceGroupName, storageAccountName)
	if err != nil {
		tel.LogError(ctx, err, OperationDeleteAccount, AzureErrorMetrics{
			ErrorType:      "StorageAccountDeletionError",
			Classification: ClassifyError(err),
			Operation:      "delete_account",
			ResourceType:   "storage_account",
			ResourceName:   storageAccountName,
			SubscriptionID: subscriptionID,
		})

		return err
	}

	// Log successful completion
	tel.LogOperation(ctx, OperationDeleteAccount, time.Since(startTime), map[string]interface{}{
		"storage_account": storageAccountName,
		"status":          "completed",
	})

	return nil
}

// bootstrapStorageAccount handles creating or checking a storage account
func (backend *Backend) bootstrapStorageAccount(ctx context.Context, l log.Logger, azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	errorHandler := backend.getErrorHandler(l)
	tel := backend.getTelemetry(l)
	startTime := time.Now()

	// Use error handler for telemetry and context
	return errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationBootstrap,
		"storage_account",
		azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		func() error {
			// Step 1: Validate storage configuration
			if err := backend.validateStorageConfig(azureCfg); err != nil {
				tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
					ErrorType:      "ConfigValidationError",
					Classification: ErrorClassConfiguration,
					Operation:      OperationBootstrap,
					ResourceType:   "storage_account",
					ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				})

				return err
			}

			// Step 2: Initialize required services from container
			storageConfig := map[string]interface{}{
				"subscription_id":      azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
				"resource_group_name":  azureCfg.StorageAccountConfig.ResourceGroupName,
				"storage_account_name": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				"location":             azureCfg.StorageAccountConfig.Location,
				"use_azuread_auth":     azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth,
				"enable_versioning":    azureCfg.StorageAccountConfig.EnableVersioning,
				"account_kind":         azureCfg.StorageAccountConfig.AccountKind,
				"account_tier":         azureCfg.StorageAccountConfig.AccountTier,
				"access_tier":          azureCfg.StorageAccountConfig.AccessTier,
				"replication_type":     azureCfg.StorageAccountConfig.ReplicationType,
				"tags":                 azureCfg.StorageAccountConfig.StorageAccountTags,
				// Skip storage account existence check during bootstrap when creating storage account
				"skip_storage_account_existence_check": true,
			}

			// Get storage account service
			storageService, err := backend.serviceContainer.GetStorageAccountService(ctx, l, storageConfig)
			if err != nil {
				tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
					ErrorType:      "ServiceCreationError",
					Classification: ErrorClassConfiguration,
					Operation:      OperationBootstrap,
					ResourceType:   "storage_account",
					ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				})

				return err
			}

			// Get blob service for access verification
			// Skip existence check since we're about to create the storage account if needed
			blobService, err := backend.serviceContainer.GetBlobService(ctx, l, storageConfig)
			if err != nil {
				tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
					ErrorType:      "ServiceCreationError",
					Classification: ErrorClassConfiguration,
					Operation:      OperationBootstrap,
					ResourceType:   "blob_service",
					ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				})

				return err
			}

			// Step 3: Ensure storage account exists (creates if needed)
			if err := backend.ensureStorageAccountExists(ctx, l, storageService, azureCfg); err != nil {
				tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
					ErrorType:      "StorageAccountCreationError",
					Classification: ClassifyError(err),
					Operation:      OperationBootstrap,
					ResourceType:   "storage_account",
					ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
					Location:       azureCfg.StorageAccountConfig.Location,
				})

				return err
			}

			// Step 4: Configure RBAC roles if using Azure AD auth
			if azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth {
				// Get the RBAC service from the container with the subscription ID
				rbacService, err := backend.serviceContainer.GetRBACService(ctx, l, map[string]interface{}{
					"subscriptionId": azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
				})
				if err != nil {
					l.Warnf("Failed to get RBAC service: %v", err)
					// Log telemetry for RBAC service creation failure
					tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
						ErrorType:      "RBACServiceError",
						Classification: ErrorClassConfiguration,
						Operation:      OperationBootstrap,
						ResourceType:   "rbac",
						ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
					})
				} else {
					// Try to assign roles but don't fail if it doesn't work
					if err := backend.assignRBACRolesWithService(ctx, l, rbacService, storageService, azureCfg); err != nil {
						l.Warnf("Failed to assign RBAC roles: %v", err)
						// Don't fail the bootstrap process for RBAC errors
						tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
							ErrorType:      "RBACAssignmentError",
							Classification: ErrorClassPermissions,
							Operation:      OperationBootstrap,
							ResourceType:   "rbac",
							ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
						})
					}
				}
			}

			l.Infof("Storage account %s exists and is configured", azureCfg.RemoteStateConfigAzurerm.StorageAccountName)

			// Step 5: Verify we can access the storage account
			if err := backend.verifyStorageAccess(ctx, l, blobService, azureCfg); err != nil {
				tel.LogErrorWithMetrics(ctx, err, OperationBootstrap, AzureErrorMetrics{
					ErrorType:      "AccessVerificationError",
					Classification: ClassifyError(err),
					Operation:      OperationBootstrap,
					ResourceType:   "storage_account",
					ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				})

				return err
			}

			// Log successful completion
			tel.LogOperation(ctx, OperationBootstrap, time.Since(startTime), map[string]interface{}{
				"storage_account": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
				"resource_group":  azureCfg.StorageAccountConfig.ResourceGroupName,
				"location":        azureCfg.StorageAccountConfig.Location,
				"status":          "completed",
			})

			return nil
		},
	)
}

// convertToStorageAccountConfig converts an ExtendedRemoteStateConfigAzurerm to a StorageAccountConfig.
func (backend *Backend) convertToStorageAccountConfig(azureCfg *ExtendedRemoteStateConfigAzurerm) *types.StorageAccountConfig {
	return &types.StorageAccountConfig{
		Name:                  azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		ResourceGroupName:     azureCfg.StorageAccountConfig.ResourceGroupName,
		Location:              azureCfg.StorageAccountConfig.Location,
		EnableVersioning:      azureCfg.StorageAccountConfig.EnableVersioning,
		AllowBlobPublicAccess: !azureCfg.DisableBlobPublicAccess && azureCfg.StorageAccountConfig.AllowBlobPublicAccess,
		AccountKind:           types.AccountKind(azureCfg.StorageAccountConfig.AccountKind),
		AccountTier:           types.AccountTier(azureCfg.StorageAccountConfig.AccountTier),
		AccessTier:            types.AccessTier(azureCfg.StorageAccountConfig.AccessTier),
		ReplicationType:       types.ReplicationType(azureCfg.StorageAccountConfig.ReplicationType),
		Tags:                  azureCfg.StorageAccountConfig.StorageAccountTags,
	}
}

// prepareServiceConfig creates a configuration map for service initialization.
func (backend *Backend) prepareServiceConfig(azureCfg *ExtendedRemoteStateConfigAzurerm, opts *options.TerragruntOptions) map[string]interface{} {
	config := map[string]interface{}{
		"subscription_id":          azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"resource_group_name":      azureCfg.StorageAccountConfig.ResourceGroupName,
		"storage_account_name":     azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"location":                 azureCfg.StorageAccountConfig.Location,
		"use_azuread_auth":         azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth,
		"use_msi":                  azureCfg.RemoteStateConfigAzurerm.UseMsi,
		"account_kind":             azureCfg.StorageAccountConfig.AccountKind,
		"account_tier":             azureCfg.StorageAccountConfig.AccountTier,
		"access_tier":              azureCfg.StorageAccountConfig.AccessTier,
		"replication_type":         azureCfg.StorageAccountConfig.ReplicationType,
		"versioning_enabled":       azureCfg.StorageAccountConfig.EnableVersioning,
		"allow_blob_public_access": !azureCfg.DisableBlobPublicAccess && azureCfg.StorageAccountConfig.AllowBlobPublicAccess,
		"tags":                     azureCfg.StorageAccountConfig.StorageAccountTags,
	}

	// Add authentication related fields if present
	if azureCfg.RemoteStateConfigAzurerm.ClientID != "" {
		config["client_id"] = azureCfg.RemoteStateConfigAzurerm.ClientID
	}

	if azureCfg.RemoteStateConfigAzurerm.ClientSecret != "" {
		config["client_secret"] = azureCfg.RemoteStateConfigAzurerm.ClientSecret
	}

	if azureCfg.RemoteStateConfigAzurerm.TenantID != "" {
		config["tenant_id"] = azureCfg.RemoteStateConfigAzurerm.TenantID
	}

	if azureCfg.RemoteStateConfigAzurerm.SasToken != "" {
		config["sas_token"] = azureCfg.RemoteStateConfigAzurerm.SasToken
	}

	// Add TerragruntOptions if provided
	if opts != nil {
		config["terragrunt_opts"] = opts
	}

	return config
}

// prepareStorageServiceConfig creates a configuration map specifically for storage services.
func (backend *Backend) prepareStorageServiceConfig(azureCfg *ExtendedRemoteStateConfigAzurerm) map[string]interface{} {
	config := backend.prepareServiceConfig(azureCfg, nil)
	config["create_if_not_exists"] = azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists
	config["skip_account_update"] = azureCfg.StorageAccountConfig.SkipStorageAccountUpdate

	return config
}

// prepareBlobServiceConfig creates a configuration map specifically for blob services.
func (backend *Backend) prepareBlobServiceConfig(azureCfg *ExtendedRemoteStateConfigAzurerm, opts *options.TerragruntOptions) map[string]interface{} {
	config := backend.prepareServiceConfig(azureCfg, opts)
	config["container_name"] = azureCfg.RemoteStateConfigAzurerm.ContainerName

	return config
}

// getStorageAccountService creates a StorageAccountService with proper configuration.
func (backend *Backend) getStorageAccountService(ctx context.Context, l log.Logger, azureCfg *ExtendedRemoteStateConfigAzurerm) (interfaces.StorageAccountService, error) {
	config := backend.prepareStorageServiceConfig(azureCfg)
	return backend.serviceContainer.GetStorageAccountService(ctx, l, config)
}

// getBlobService creates a BlobService with proper configuration.
func (backend *Backend) getBlobService(ctx context.Context, l log.Logger, azureCfg *ExtendedRemoteStateConfigAzurerm, opts *options.TerragruntOptions) (interfaces.BlobService, error) {
	config := backend.prepareBlobServiceConfig(azureCfg, opts)
	return backend.serviceContainer.GetBlobService(ctx, l, config)
}

// validateStorageConfig validates the storage account configuration.
func (backend *Backend) validateStorageConfig(azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	// Convert the backend config to a storage account config
	storageConfig := backend.convertToStorageAccountConfig(azureCfg)

	// Validate required fields
	if storageConfig.Name == "" {
		return WrapValidationError("storage account name is required")
	}

	if storageConfig.ResourceGroupName == "" && azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
		return WrapValidationError("resource group name is required when CreateStorageAccountIfNotExists is true")
	}

	// Validate storage account configuration
	if err := ValidateStorageAccountName(storageConfig.Name); err != nil {
		return err
	}

	return nil
}

// ensureStorageAccountExists ensures that a storage account exists with the specified configuration.
func (backend *Backend) ensureStorageAccountExists(ctx context.Context, l log.Logger, storageService interfaces.StorageAccountService, azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	// Convert the backend config to a storage account config
	storageConfig := backend.convertToStorageAccountConfig(azureCfg)

	// Check if the storage account exists
	account, err := storageService.GetStorageAccount(ctx, storageConfig.ResourceGroupName, storageConfig.Name)
	if err != nil {
		return backend.wrapStorageAccountError(err, storageConfig.Name)
	}

	if account == nil {
		if !azureCfg.StorageAccountConfig.CreateStorageAccountIfNotExists {
			return WrapStorageAccountNotFoundError(storageConfig.Name)
		}

		// Create the storage account
		l.Infof("Creating Azure Storage Account %s in resource group %s", storageConfig.Name, storageConfig.ResourceGroupName)

		if err := storageService.CreateStorageAccount(ctx, storageConfig); err != nil {
			return backend.wrapStorageAccountError(err, storageConfig.Name)
		}
	}

	return nil
}

// verifyStorageAccess verifies access to the storage account by attempting to perform basic operations.
func (backend *Backend) verifyStorageAccess(ctx context.Context, l log.Logger, blobService interfaces.BlobService, azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	containerName := azureCfg.RemoteStateConfigAzurerm.ContainerName
	testBlobName := ".terragrunt-test-blob"

	// Try to create the container if it doesn't exist
	if err := blobService.CreateContainerIfNecessary(ctx, l, containerName); err != nil {
		return backend.wrapBlobError(err, containerName, testBlobName)
	}

	// Try to upload a test blob
	testData := []byte("Terragrunt storage access test")
	if err := blobService.UploadBlob(ctx, l, containerName, testBlobName, testData); err != nil {
		return backend.wrapBlobError(err, containerName, testBlobName)
	}

	// Clean up the test blob
	if err := blobService.DeleteBlobIfNecessary(ctx, l, containerName, testBlobName); err != nil {
		l.Warnf("Failed to delete test blob %s in container %s: %v", testBlobName, containerName, err)
	}

	return nil
}

// deleteStorageAccountWithRetry attempts to delete a storage account with retry logic.
func (backend *Backend) deleteStorageAccountWithRetry(ctx context.Context, l log.Logger, storageService interfaces.StorageAccountService, resourceGroupName, storageAccountName string) error {
	retryConfig := DefaultRetryConfig()

	return WithRetry(ctx, l, "storage account deletion", retryConfig, func() error {
		err := storageService.DeleteStorageAccount(ctx, resourceGroupName, storageAccountName)
		if err != nil {
			return WrapTransientError(err, "storage account deletion")
		}

		return nil
	})
}

// Error wrapper functions for storage account operations

// WrapValidationError wraps a storage account validation error with context.
func WrapValidationError(msg string) error {
	return fmt.Errorf("storage account validation error: %s", msg)
}

// WrapStorageAccountNotFoundError wraps a storage account not found error.
func WrapStorageAccountNotFoundError(accountName string) error {
	return fmt.Errorf("storage account %s not found and CreateStorageAccountIfNotExists is false", accountName)
}

// ValidateStorageAccountName validates an Azure Storage account name.
func ValidateStorageAccountName(name string) error {
	if name == "" {
		return WrapValidationError("storage account name cannot be empty")
	}

	if len(name) < 3 || len(name) > 24 {
		return WrapValidationError("storage account name must be between 3 and 24 characters")
	}

	// Storage account names must only contain lowercase letters and numbers
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return WrapValidationError("storage account name can only contain lowercase letters and numbers")
		}
	}

	return nil
}

// Helper methods for backend

// getTelemetry returns the telemetry collector, initializing it if needed
func (backend *Backend) getTelemetry(l log.Logger) *AzureTelemetryCollector {
	if backend.telemetry == nil {
		// Check if we have enhanced telemetry settings
		enableDetailedMetrics := true
		metricsBufferSize := 1000

		if factory, ok := backend.serviceFactory.(*enhancedServiceFactory); ok {
			if factory.telemetrySettings != nil {
				enableDetailedMetrics = factory.telemetrySettings.EnableDetailedMetrics
				metricsBufferSize = factory.telemetrySettings.MetricsBufferSize
			}
		}

		// Create telemetry collector with enhanced settings
		backend.telemetry = NewAzureTelemetryCollectorWithSettings(l, &TelemetryCollectorSettings{
			EnableDetailedMetrics: enableDetailedMetrics,
			BufferSize:            metricsBufferSize,
		})
	}

	return backend.telemetry
}

// getErrorHandler returns the error handler, initializing it if needed
func (backend *Backend) getErrorHandler(l log.Logger) *azureutil.ErrorHandler {
	if backend.errorHandler == nil {
		tel := backend.getTelemetry(l)
		telemetryAdapter := NewTelemetryAdapter(tel, l)
		backend.errorHandler = azureutil.NewErrorHandler(telemetryAdapter, l)
	}

	return backend.errorHandler
}

// initTelemetry initializes telemetry collection
func (backend *Backend) initTelemetry(l log.Logger) {
	tel := backend.getTelemetry(l)
	// Telemetry is initialized when created
	_ = tel
}

// logServiceOperation logs telemetry for a service operation
func (backend *Backend) logServiceOperation(ctx context.Context, l log.Logger, operation OperationType, duration time.Duration, resourceType, resourceName string, err error) {
	tel := backend.getTelemetry(l)

	attrs := map[string]interface{}{
		"resource_type": resourceType,
		"resource_name": resourceName,
		"duration_ms":   duration.Milliseconds(),
	}

	if err != nil {
		// Create error metrics for telemetry
		metrics := AzureErrorMetrics{
			ErrorType:      "unknown",         // Will be determined by telemetry
			Classification: ErrorClassUnknown, // Will be classified by telemetry
			Operation:      operation,
			ResourceType:   resourceType,
			ResourceName:   resourceName,
			Duration:       duration,
			ErrorMessage:   err.Error(),
			IsRetryable:    false, // Conservative default
		}

		tel.LogError(ctx, err, operation, metrics)
	} else {
		// Log successful operation
		tel.LogOperation(ctx, operation, duration, attrs)
	}
}

// wrapServiceCall wraps a service call with telemetry and error handling
func (backend *Backend) wrapServiceCall(ctx context.Context, l log.Logger, operation OperationType, resourceType, resourceName string, fn func() error) error {
	startTime := time.Now()
	err := fn()
	duration := time.Since(startTime)

	backend.logServiceOperation(ctx, l, operation, duration, resourceType, resourceName, err)

	return err
}

// enhancedServiceFactory wraps a service container with enhanced configuration
type enhancedServiceFactory struct {
	container         interfaces.AzureServiceContainer
	telemetrySettings *TelemetrySettings
	authSettings      *AuthSettings
}

// CreateContainer implements interfaces.ServiceFactory
func (f *enhancedServiceFactory) CreateContainer(ctx context.Context) interfaces.AzureServiceContainer {
	return f.container
}

// Options implements interfaces.ServiceFactory
func (f *enhancedServiceFactory) Options() *interfaces.FactoryOptions {
	return &interfaces.FactoryOptions{
		EnableMocking: false,
		// Add other options as needed
	}
}

// applyDefaultBackendConfig applies default configuration values if not provided
func applyDefaultBackendConfig(cfg *BackendConfig) {
	// Set default retry configuration
	if cfg.RetryConfig == nil {
		cfg.RetryConfig = &interfaces.RetryConfig{
			MaxRetries: defaultRetryMaxAttempts,
			RetryDelay: defaultRetryDelaySeconds,
			MaxDelay:   defaultMaxDelaySeconds,
			RetryableStatusCodes: []int{
				http.StatusRequestTimeout,
				http.StatusTooManyRequests,
				http.StatusInternalServerError,
				http.StatusBadGateway,
				http.StatusServiceUnavailable,
				http.StatusGatewayTimeout,
			},
		}
	}

	// Set default telemetry settings
	if cfg.TelemetrySettings == nil {
		cfg.TelemetrySettings = &TelemetrySettings{
			EnableDetailedMetrics: true,
			EnableErrorTracking:   true,
			MetricsBufferSize:     defaultMetricsBufferSize,
			FlushInterval:         defaultFlushIntervalSeconds,
		}
	}

	// Set default cache settings
	if cfg.CacheSettings == nil {
		cfg.CacheSettings = &CacheSettings{
			EnableCaching:      true,
			CacheTimeout:       defaultCacheTimeoutSeconds, // 5 minutes
			MaxCacheSize:       defaultMaxCacheSize,
			EnableCacheMetrics: false,
		}
	}

	// Set default auth settings
	if cfg.AuthSettings == nil {
		cfg.AuthSettings = &AuthSettings{
			PreferredAuthMethod: interfaces.DefaultAuthenticationConfig().Method, // Azure AD is the preferred method
			EnableAuthCaching:   true,
			AuthCacheTimeout:    defaultAuthCacheTimeoutSecs, // 1 hour
			EnableAuthRetry:     true,
		}
	}
}

// createFactoryOptionsFromConfig creates factory options based on backend configuration
func createFactoryOptionsFromConfig(cfg *BackendConfig) *interfaces.FactoryOptions {
	options := &interfaces.FactoryOptions{
		EnableMocking: false, // Production default
		RetryConfig:   cfg.RetryConfig,
	}

	// Configure default config map based on settings
	defaultConfig := make(map[string]interface{})

	if cfg.AuthSettings != nil {
		defaultConfig["preferred_auth_method"] = cfg.AuthSettings.PreferredAuthMethod
		defaultConfig["enable_auth_caching"] = cfg.AuthSettings.EnableAuthCaching
		defaultConfig["auth_cache_timeout"] = cfg.AuthSettings.AuthCacheTimeout
		defaultConfig["enable_auth_retry"] = cfg.AuthSettings.EnableAuthRetry
	}

	if cfg.CacheSettings != nil {
		defaultConfig["enable_caching"] = cfg.CacheSettings.EnableCaching
		defaultConfig["cache_timeout"] = cfg.CacheSettings.CacheTimeout
		defaultConfig["max_cache_size"] = cfg.CacheSettings.MaxCacheSize
		defaultConfig["enable_cache_metrics"] = cfg.CacheSettings.EnableCacheMetrics
	}

	if cfg.TelemetrySettings != nil {
		defaultConfig["enable_detailed_metrics"] = cfg.TelemetrySettings.EnableDetailedMetrics
		defaultConfig["enable_error_tracking"] = cfg.TelemetrySettings.EnableErrorTracking
		defaultConfig["metrics_buffer_size"] = cfg.TelemetrySettings.MetricsBufferSize
		defaultConfig["flush_interval"] = cfg.TelemetrySettings.FlushInterval
	}

	options.DefaultConfig = defaultConfig

	return options
}

// validateDeleteStorageAccountConfig validates configuration for storage account deletion
func (backend *Backend) validateDeleteStorageAccountConfig(resourceGroupName, subscriptionID string) error {
	if resourceGroupName == "" {
		return MissingResourceGroupError{}
	}

	if subscriptionID == "" {
		return MissingSubscriptionIDError{}
	}

	return nil
}

// promptForStorageAccountDeletion prompts user for confirmation before deletion
func (backend *Backend) promptForStorageAccountDeletion(ctx context.Context, l log.Logger, storageAccountName string, opts *options.TerragruntOptions) (bool, error) {
	if opts.NonInteractive {
		return false, fmt.Errorf("cannot delete storage account %s in non-interactive mode: user confirmation is required", storageAccountName)
	}

	prompt := fmt.Sprintf("Are you sure you want to delete storage account %s? This action cannot be undone. (y/N)", storageAccountName)

	response, err := shell.PromptUserForInput(ctx, l, prompt, opts)
	if err != nil {
		return false, fmt.Errorf("failed to get user confirmation: %w", err)
	}

	return strings.ToLower(strings.TrimSpace(response)) == "y", nil
}

// GetBlobServiceFromConfig creates a blob service from the config
func (backend *Backend) GetBlobServiceFromConfig(ctx context.Context, l log.Logger, azureCfg *ExtendedRemoteStateConfigAzurerm) (interfaces.BlobService, error) {
	// Convert config to map for service container
	config := map[string]interface{}{
		"storage_account_name": azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"container_name":       azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"resource_group_name":  azureCfg.RemoteStateConfigAzurerm.ResourceGroupName,
		"subscription_id":      azureCfg.RemoteStateConfigAzurerm.SubscriptionID,
		"tenant_id":            azureCfg.RemoteStateConfigAzurerm.TenantID,
		"client_id":            azureCfg.RemoteStateConfigAzurerm.ClientID,
		"client_secret":        azureCfg.RemoteStateConfigAzurerm.ClientSecret,
		"use_azuread_auth":     azureCfg.RemoteStateConfigAzurerm.UseAzureADAuth,
		"use_msi":              azureCfg.RemoteStateConfigAzurerm.UseMsi,
	}

	return backend.serviceContainer.GetBlobService(ctx, l, config)
}

// GetObject retrieves an object from Azure blob storage
func (backend *Backend) GetObject(ctx context.Context, l log.Logger, containerName, blobKey string, blobService interfaces.BlobService) ([]byte, error) {
	input := &types.GetObjectInput{
		ContainerName: containerName,
		BlobName:      blobKey,
	}

	var result []byte

	err := backend.wrapServiceCall(ctx, l, OperationBlobGet, "blob", blobKey, func() error {
		output, err := blobService.GetObject(ctx, input)
		if err != nil {
			return err
		}

		result = output.Content

		return nil
	})

	return result, err
}

// assignRBACRolesWithService assigns RBAC roles using the provided services
func (backend *Backend) assignRBACRolesWithService(ctx context.Context, l log.Logger, rbacService interfaces.RBACService, storageService interfaces.StorageAccountService, azureCfg *ExtendedRemoteStateConfigAzurerm) error {
	// Get the storage account details to verify it exists
	resourceGroupName := azureCfg.StorageAccountConfig.ResourceGroupName
	storageAccountName := azureCfg.RemoteStateConfigAzurerm.StorageAccountName

	if resourceGroupName == "" {
		l.Debugf("Resource group name not available, skipping RBAC role assignment")
		return nil
	}

	l.Debugf("Verifying storage account exists for RBAC role assignment")
	storageAccount, err := storageService.GetStorageAccount(ctx, resourceGroupName, storageAccountName)
	if err != nil {
		return fmt.Errorf("failed to get storage account details: %w", err)
	}

	if storageAccount == nil {
		return errors.New("storage account is nil")
	}

	// Construct Azure resource ID for the storage account
	// Format: /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Storage/storageAccounts/{accountName}
	subscriptionID := azureCfg.RemoteStateConfigAzurerm.SubscriptionID
	scope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		subscriptionID, resourceGroupName, storageAccountName)

	l.Debugf("Assigning Storage Account Contributor role at scope: %s", scope)

	// Assign Storage Account Contributor role to the current principal
	if err := rbacService.AssignStorageBlobDataOwnerRole(ctx, l, scope); err != nil {
		// Check if it's a permission error (user doesn't have rights to assign roles)
		if rbacService.IsPermissionError(err) {
			l.Warnf("Insufficient permissions to assign RBAC roles. You may need to manually assign 'Storage Account Contributor' role.")
			return nil // Don't fail bootstrap for permission errors
		}

		// Check if role is already assigned (not an error)
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "RoleAssignmentExists") {
			l.Infof("Storage Account Contributor role is already assigned to current principal")
			return nil
		}

		return fmt.Errorf("failed to assign Storage Account Contributor role: %w", err)
	}

	l.Infof("Successfully assigned Storage Account Contributor role to current principal")

	return nil
}

// GetFactoryConfiguration returns the current factory configuration
func (backend *Backend) GetFactoryConfiguration() map[string]interface{} {
	if factory, ok := backend.serviceFactory.(*enhancedServiceFactory); ok {
		config := make(map[string]interface{})

		if factory.telemetrySettings != nil {
			config["telemetry"] = map[string]interface{}{
				"enable_detailed_metrics": factory.telemetrySettings.EnableDetailedMetrics,
				"enable_error_tracking":   factory.telemetrySettings.EnableErrorTracking,
				"metrics_buffer_size":     factory.telemetrySettings.MetricsBufferSize,
				"flush_interval":          factory.telemetrySettings.FlushInterval,
			}
		}

		if factory.authSettings != nil {
			config["auth"] = map[string]interface{}{
				"preferred_auth_method": factory.authSettings.PreferredAuthMethod,
				"enable_auth_caching":   factory.authSettings.EnableAuthCaching,
				"auth_cache_timeout":    factory.authSettings.AuthCacheTimeout,
				"enable_auth_retry":     factory.authSettings.EnableAuthRetry,
			}
		}

		return config
	}

	return make(map[string]interface{})
}

// UpdateFactoryConfiguration updates the factory configuration at runtime
func (backend *Backend) UpdateFactoryConfiguration(updates map[string]interface{}) error {
	if factory, ok := backend.serviceFactory.(*enhancedServiceFactory); ok {
		// Update telemetry settings
		if telemetryUpdates, exists := updates["telemetry"].(map[string]interface{}); exists {
			if factory.telemetrySettings != nil {
				if enableMetrics, ok := telemetryUpdates["enable_detailed_metrics"].(bool); ok {
					factory.telemetrySettings.EnableDetailedMetrics = enableMetrics
				}

				if enableTracking, ok := telemetryUpdates["enable_error_tracking"].(bool); ok {
					factory.telemetrySettings.EnableErrorTracking = enableTracking
				}

				if bufferSize, ok := telemetryUpdates["metrics_buffer_size"].(int); ok {
					factory.telemetrySettings.MetricsBufferSize = bufferSize
				}

				if flushInterval, ok := telemetryUpdates["flush_interval"].(int); ok {
					factory.telemetrySettings.FlushInterval = flushInterval
				}
			}
		}

		// Update auth settings
		if authUpdates, exists := updates["auth"].(map[string]interface{}); exists {
			if factory.authSettings != nil {
				if authMethod, ok := authUpdates["preferred_auth_method"].(string); ok {
					factory.authSettings.PreferredAuthMethod = authMethod
				}

				if enableCaching, ok := authUpdates["enable_auth_caching"].(bool); ok {
					factory.authSettings.EnableAuthCaching = enableCaching
				}

				if cacheTimeout, ok := authUpdates["auth_cache_timeout"].(int); ok {
					factory.authSettings.AuthCacheTimeout = cacheTimeout
				}

				if enableRetry, ok := authUpdates["enable_auth_retry"].(bool); ok {
					factory.authSettings.EnableAuthRetry = enableRetry
				}
			}
		}

		return nil
	}

	return errors.New("backend does not use enhanced service factory")
}

// IsVersionControlEnabled returns true if blob versioning is enabled for the Azure storage account.
func (backend *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	startTime := time.Now()
	tel := backend.getTelemetry(l)
	errorHandler := backend.getErrorHandler(l)

	// Parse the Azure configuration with error handling
	var azureCfg *ExtendedRemoteStateConfigAzurerm

	err := errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationStorageOp,
		"config",
		"azurerm",
		func() error {
			var configErr error

			azureCfg, configErr = Config(backendConfig).ExtendedAzureConfig()

			return configErr
		},
	)
	if err != nil {
		tel.LogError(ctx, err, OperationVersionCheck, AzureErrorMetrics{
			ErrorType:      "ConfigError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationVersionCheck,
		})

		return false, err
	}

	// Validate container name before any Azure operations
	containerName := azureCfg.RemoteStateConfigAzurerm.ContainerName

	err = errorHandler.WithErrorHandling(
		ctx,
		azureutil.OperationValidation,
		"container",
		containerName,
		func() error {
			return ValidateContainerName(containerName)
		},
	)
	if err != nil {
		tel.LogError(ctx, err, OperationVersionCheck, AzureErrorMetrics{
			ErrorType:      "ValidationError",
			Classification: ErrorClassValidation,
			Operation:      OperationVersionCheck,
			ResourceType:   "container",
			ResourceName:   containerName,
		})

		return false, err
	}

	// Get the storage account service to check versioning
	storageService, err := backend.getStorageAccountService(ctx, l, azureCfg)
	if err != nil {
		tel.LogError(ctx, err, OperationVersionCheck, AzureErrorMetrics{
			ErrorType:      "ServiceCreationError",
			Classification: ErrorClassConfiguration,
			Operation:      OperationVersionCheck,
			ResourceType:   "storage_account",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		})

		return false, err
	}

	// Check if versioning is enabled using retry logic
	var versioningEnabled bool

	retryConfig := DefaultRetryConfig()

	err = WithRetry(ctx, l, "versioning check", retryConfig, func() error {
		enabled, versionErr := storageService.IsVersioningEnabled(ctx)
		if versionErr != nil {
			return WrapTransientError(versionErr, "versioning check")
		}

		versioningEnabled = enabled

		return nil
	})
	if err != nil {
		tel.LogError(ctx, err, OperationVersionCheck, AzureErrorMetrics{
			ErrorType:      "VersioningCheckError",
			Classification: ClassifyError(err),
			Operation:      OperationVersionCheck,
			ResourceType:   "storage_account",
			ResourceName:   azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		})

		return false, err
	}

	// Log successful completion
	tel.LogOperation(ctx, OperationVersionCheck, time.Since(startTime), map[string]interface{}{
		"storage_account":    azureCfg.RemoteStateConfigAzurerm.StorageAccountName,
		"container":          azureCfg.RemoteStateConfigAzurerm.ContainerName,
		"versioning_enabled": versioningEnabled,
		"status":             "completed",
	})

	return versioningEnabled, nil
}
