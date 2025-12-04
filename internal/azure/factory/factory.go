// Package factory provides factory functions for creating Azure service implementations
package factory

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/implementations"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Options for configuring the enhanced service factory
type Options struct {
	MockResponses map[string]interface{}
	DefaultConfig map[string]interface{}
	RetryConfig   *interfaces.RetryConfig
	EnableMocking bool
}

// AzureServiceFactory implements interfaces.AzureServiceContainer and provides
// factory methods for creating Azure services
//
//nolint:govet // fieldalignment: Struct embeds multiple maps/interfaces; reordering would hurt readability without measurable benefit.
type AzureServiceFactory struct {
	// Synchronization for caches and configuration
	cacheMutex sync.RWMutex

	// Configuration
	config  interfaces.ServiceContainerConfig
	options *interfaces.FactoryOptions

	// Service caches
	storageAccountServiceCache map[string]interfaces.StorageAccountService
	blobServiceCache           map[string]interfaces.BlobService
	resourceGroupServiceCache  map[string]interfaces.ResourceGroupService
	rbacServiceCache           map[string]interfaces.RBACService
	authServiceCache           map[string]interfaces.AuthenticationService
	cacheTimestamps            map[string]time.Time

	// Registered custom services
	customStorageAccountService interfaces.StorageAccountService
	customBlobService           interfaces.BlobService
	customResourceGroupService  interfaces.ResourceGroupService
	customRBACService           interfaces.RBACService
	customAuthService           interfaces.AuthenticationService
}

// NewAzureServiceFactory creates a new factory instance
func NewAzureServiceFactory() interfaces.AzureServiceContainer {
	return &AzureServiceFactory{
		config:                     interfaces.DefaultServiceContainerConfig(),
		options:                    nil, // Will be set via NewAzureServiceFactoryWithOptions if needed
		storageAccountServiceCache: make(map[string]interfaces.StorageAccountService),
		blobServiceCache:           make(map[string]interfaces.BlobService),
		resourceGroupServiceCache:  make(map[string]interfaces.ResourceGroupService),
		rbacServiceCache:           make(map[string]interfaces.RBACService),
		authServiceCache:           make(map[string]interfaces.AuthenticationService),
		cacheTimestamps:            make(map[string]time.Time),
	}
}

// NewAzureServiceFactoryWithOptions creates a new factory with specific options
func NewAzureServiceFactoryWithOptions(options *interfaces.FactoryOptions) interfaces.AzureServiceContainer {
	factory := NewAzureServiceFactory()

	if options != nil {
		// Apply options to the factory
		if factoryImpl, ok := factory.(*AzureServiceFactory); ok {
			// Store options for later use
			factoryImpl.options = options

			// Note: RetryConfig and DefaultConfig are accessed via factoryImpl.options
			// by service implementations when they need retry settings or default values
		}
	}

	return factory
}

// CreateContainer creates a new AzureServiceContainer instance
// ctx is reserved for future use; required by interface
func (f *AzureServiceFactory) CreateContainer(ctx context.Context) interfaces.AzureServiceContainer {
	// In this implementation, the factory itself is the container
	return f
}

// Options returns the factory options
func (f *AzureServiceFactory) Options() *interfaces.FactoryOptions {
	if f.options != nil {
		return f.options
	}
	// Return default options if none were set
	return &interfaces.FactoryOptions{
		DefaultConfig: make(map[string]interface{}),
		RetryConfig:   &interfaces.RetryConfig{},
	}
}

// Initialize initializes the service container with the provided configuration
func (f *AzureServiceFactory) Initialize(ctx context.Context, l log.Logger, config map[string]interface{}) error {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Update configuration from provided config map
	if enableCaching, ok := config["enable_caching"].(bool); ok {
		f.config.EnableCaching = enableCaching
	}

	if cacheTimeout, ok := config["cache_timeout"].(int); ok {
		f.config.CacheTimeout = cacheTimeout
	}

	if maxCacheSize, ok := config["max_cache_size"].(int); ok {
		f.config.MaxCacheSize = maxCacheSize
	}

	l.Debugf("Azure service factory initialized with config: enableCaching=%v",
		f.config.EnableCaching)

	return nil
}

// Cleanup cleans up resources held by the service container
func (f *AzureServiceFactory) Cleanup(ctx context.Context, l log.Logger) error {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Clear all caches
	f.storageAccountServiceCache = make(map[string]interfaces.StorageAccountService)
	f.blobServiceCache = make(map[string]interfaces.BlobService)
	f.resourceGroupServiceCache = make(map[string]interfaces.ResourceGroupService)
	f.rbacServiceCache = make(map[string]interfaces.RBACService)
	f.authServiceCache = make(map[string]interfaces.AuthenticationService)
	f.cacheTimestamps = make(map[string]time.Time)

	return nil
}

// Health checks the health of all services in the container
// ctx is reserved for future use; required by interface
func (f *AzureServiceFactory) Health(ctx context.Context, l log.Logger) error {
	// For now just return success
	return nil
}

// Reset resets the service container to its initial state
func (f *AzureServiceFactory) Reset(ctx context.Context, l log.Logger) error {
	return f.Cleanup(ctx, l)
}

// getCacheKey generates a cache key from the configuration
func (f *AzureServiceFactory) getCacheKey(config map[string]interface{}) string {
	// Create a simple cache key based on key configuration parameters
	// Empty strings are acceptable defaults for missing config values
	var storageAccount, subscriptionID, resourceGroup string

	if v, ok := config["storage_account_name"].(string); ok {
		storageAccount = v
	}

	if v, ok := config["subscription_id"].(string); ok {
		subscriptionID = v
	}

	if v, ok := config["resource_group_name"].(string); ok {
		resourceGroup = v
	}

	// Use structured format with URL encoding to prevent collisions
	// e.g., "a-b" + "-" + "c" vs "a" + "-" + "b-c"
	return fmt.Sprintf("sa:%s|sub:%s|rg:%s",
		url.QueryEscape(storageAccount),
		url.QueryEscape(subscriptionID),
		url.QueryEscape(resourceGroup))
}

// isExpired checks if a cache entry is expired
func (f *AzureServiceFactory) isExpired(key string) bool {
	if !f.config.EnableCaching {
		return true
	}

	timestamp, exists := f.cacheTimestamps[key]
	if !exists {
		return true
	}

	return time.Since(timestamp) > time.Duration(f.config.CacheTimeout)*time.Second
}

// GetStorageAccountService creates and returns a StorageAccountService instance
func (f *AzureServiceFactory) GetStorageAccountService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.StorageAccountService, error) {
	f.cacheMutex.RLock()

	// Check if a custom service is registered
	if f.customStorageAccountService != nil {
		f.cacheMutex.RUnlock()
		return f.customStorageAccountService, nil
	}

	// Generate cache key
	cacheKey := f.getCacheKey(config)

	// Check cache if enabled
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.storageAccountServiceCache[cacheKey]; exists {
			f.cacheMutex.RUnlock()
			return service, nil
		}
	}

	f.cacheMutex.RUnlock()

	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Check again after getting write lock (double-check pattern)
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.storageAccountServiceCache[cacheKey]; exists {
			return service, nil
		}
	}

	// Create a new storage account client
	storageAccountClient, err := azurehelper.CreateStorageAccountClient(ctx, l, config)
	if err != nil {
		return nil, errors.Errorf("failed to create storage account client: %w", err)
	}

	// Create the adapter service implementation
	service := &storageAccountServiceAdapter{
		client: storageAccountClient,
	}

	// Cache the service if caching is enabled
	if f.config.EnableCaching {
		f.storageAccountServiceCache[cacheKey] = service
		f.cacheTimestamps[cacheKey] = time.Now()
	}

	return service, nil
}

// GetBlobService creates and returns a BlobService instance
func (f *AzureServiceFactory) GetBlobService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.BlobService, error) {
	f.cacheMutex.RLock()

	// Check if a custom service is registered
	if f.customBlobService != nil {
		f.cacheMutex.RUnlock()
		return f.customBlobService, nil
	}

	// Generate cache key
	cacheKey := f.getCacheKey(config)

	// Check cache if enabled
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.blobServiceCache[cacheKey]; exists {
			f.cacheMutex.RUnlock()
			return service, nil
		}
	}

	f.cacheMutex.RUnlock()

	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Check again after getting write lock (double-check pattern)
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.blobServiceCache[cacheKey]; exists {
			return service, nil
		}
	}

	// Extract TerragruntOptions from config if available
	var terragruntOpts *options.TerragruntOptions
	if opts, ok := config["terragrunt_opts"].(*options.TerragruntOptions); ok {
		terragruntOpts = opts
	}

	// Create a new blob service client
	blobClient, err := azurehelper.CreateBlobServiceClient(ctx, l, terragruntOpts, config)
	if err != nil {
		return nil, errors.Errorf("failed to create blob service client: %w", err)
	}

	// Create the adapter service implementation
	service := &blobServiceAdapter{
		client: blobClient,
	}

	// Cache the service if caching is enabled
	if f.config.EnableCaching {
		f.blobServiceCache[cacheKey] = service
		f.cacheTimestamps[cacheKey] = time.Now()
	}

	return service, nil
}

// GetResourceGroupService creates and returns a ResourceGroupService instance
func (f *AzureServiceFactory) GetResourceGroupService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.ResourceGroupService, error) {
	f.cacheMutex.RLock()

	// Check if a custom service is registered
	if f.customResourceGroupService != nil {
		f.cacheMutex.RUnlock()
		return f.customResourceGroupService, nil
	}

	// Generate cache key
	cacheKey := f.getCacheKey(config)

	// Check cache if enabled
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.resourceGroupServiceCache[cacheKey]; exists {
			f.cacheMutex.RUnlock()
			return service, nil
		}
	}

	f.cacheMutex.RUnlock()

	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Check again after getting write lock (double-check pattern)
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.resourceGroupServiceCache[cacheKey]; exists {
			return service, nil
		}
	}

	// Extract the subscription ID from config
	subscriptionID, ok := config["subscription_id"].(string)
	if !ok || subscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required in the configuration")
	}

	// Create a new resource group client
	resourceGroupClient, err := azurehelper.CreateResourceGroupClient(ctx, l, subscriptionID)
	if err != nil {
		return nil, errors.Errorf("failed to create resource group client: %w", err)
	}

	// Create the adapter service implementation
	service := &resourceGroupServiceAdapter{
		client: resourceGroupClient,
	}

	// Cache the service if caching is enabled
	if f.config.EnableCaching {
		f.resourceGroupServiceCache[cacheKey] = service
		f.cacheTimestamps[cacheKey] = time.Now()
	}

	return service, nil
}

// GetRBACService creates and returns an RBACService instance
func (f *AzureServiceFactory) GetRBACService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.RBACService, error) {
	f.cacheMutex.RLock()

	// Check if a custom service is registered
	if f.customRBACService != nil {
		f.cacheMutex.RUnlock()
		return f.customRBACService, nil
	}

	// Generate cache key
	cacheKey := f.getCacheKey(config)

	// Check cache if enabled
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.rbacServiceCache[cacheKey]; exists {
			f.cacheMutex.RUnlock()
			return service, nil
		}
	}

	f.cacheMutex.RUnlock()

	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	// Check again after getting write lock (double-check pattern)
	if f.config.EnableCaching && !f.isExpired(cacheKey) {
		if service, exists := f.rbacServiceCache[cacheKey]; exists {
			return service, nil
		}
	}

	// Create RBAC service using the production implementation
	// Extract subscription ID from config
	subscriptionID, ok := config["subscription_id"].(string)
	if !ok || subscriptionID == "" {
		// Try alternate key format
		subscriptionID, _ = config["subscriptionId"].(string)
	}

	if subscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required for RBAC operations")
	}

	// Create credential using azureauth
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, errors.Errorf("failed to get auth config: %w", err)
	}

	authResult, err := azureauth.GetTokenCredential(ctx, l, authConfig)
	if err != nil {
		return nil, errors.Errorf("failed to get token credential: %w", err)
	}

	// Create RBAC service
	rbacConfig := interfaces.DefaultRBACConfig()
	service := implementations.NewRBACService(authResult.Credential, rbacConfig, subscriptionID)

	// Cache the service if caching is enabled
	if f.config.EnableCaching {
		f.rbacServiceCache[cacheKey] = service
		f.cacheTimestamps[cacheKey] = time.Now()
	}

	return service, nil
}

// GetAuthenticationService creates and returns an AuthenticationService instance
func (f *AzureServiceFactory) GetAuthenticationService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.AuthenticationService, error) {
	// For now, we'll return a not implemented error
	return nil, errors.Errorf("AuthenticationService not implemented")
}

// RegisterStorageAccountService registers a custom StorageAccountService implementation
func (f *AzureServiceFactory) RegisterStorageAccountService(service interfaces.StorageAccountService) {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	f.customStorageAccountService = service
}

// RegisterBlobService registers a custom BlobService implementation
func (f *AzureServiceFactory) RegisterBlobService(service interfaces.BlobService) {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	f.customBlobService = service
}

// RegisterResourceGroupService registers a custom ResourceGroupService implementation
func (f *AzureServiceFactory) RegisterResourceGroupService(service interfaces.ResourceGroupService) {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	f.customResourceGroupService = service
}

// RegisterRBACService registers a custom RBACService implementation
func (f *AzureServiceFactory) RegisterRBACService(service interfaces.RBACService) {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	f.customRBACService = service
}

// RegisterAuthenticationService registers a custom AuthenticationService implementation
func (f *AzureServiceFactory) RegisterAuthenticationService(service interfaces.AuthenticationService) {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	f.customAuthService = service
}

// GetRegisteredServices returns a list of all registered service types
func (f *AzureServiceFactory) GetRegisteredServices() []string {
	services := []string{}

	if f.customStorageAccountService != nil {
		services = append(services, "storage")
	}

	if f.customBlobService != nil {
		services = append(services, "blob")
	}

	if f.customResourceGroupService != nil {
		services = append(services, "resourcegroup")
	}

	if f.customRBACService != nil {
		services = append(services, "rbac")
	}

	if f.customAuthService != nil {
		services = append(services, "auth")
	}

	return services
}

// HasService checks if a specific service type is registered
func (f *AzureServiceFactory) HasService(serviceType string) bool {
	switch serviceType {
	case "storage":
		return f.customStorageAccountService != nil
	case "blob":
		return f.customBlobService != nil
	case "resourcegroup":
		return f.customResourceGroupService != nil
	case "rbac":
		return f.customRBACService != nil
	case "auth":
		return f.customAuthService != nil
	default:
		return false
	}
}

// GetServiceInfo returns information about a specific service
func (f *AzureServiceFactory) GetServiceInfo(serviceType string) (map[string]interface{}, error) {
	info := map[string]interface{}{
		"type": serviceType,
	}

	if !f.HasService(serviceType) {
		return info, errors.Errorf("service type '%s' not registered", serviceType)
	}

	return info, nil
}
