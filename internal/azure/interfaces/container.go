// Package interfaces provides the unified service container for dependency injection
package interfaces

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// AzureServiceContainer provides a unified interface for all Azure services.
// This follows the dependency injection pattern and allows for easy testing and mocking.
// The container manages the lifecycle of Azure service instances and provides
// a consistent way to access them throughout the application.
//
// Usage examples:
//
//	// Create a service container
//	container := factory.NewAzureServiceContainer()
//
//	// Get a storage account service
//	storageService, err := container.GetStorageAccountService(ctx, logger, config)
//
//	// Get a blob service for operations
//	blobService, err := container.GetBlobService(ctx, logger, opts, config)
//

type AzureServiceContainer interface {
	// Service Factories

	// GetStorageAccountService creates and returns a StorageAccountService instance.
	// The service handles Azure Storage account operations like creation, deletion,
	// and configuration management.
	// config: Configuration map containing storage account parameters
	GetStorageAccountService(ctx context.Context, l log.Logger, config map[string]interface{}) (StorageAccountService, error)

	// GetBlobService creates and returns a BlobService instance.
	// The service handles Azure Blob Storage operations like uploading, downloading,
	// and container management.
	// config: Configuration map containing blob service parameters (can include options)
	GetBlobService(ctx context.Context, l log.Logger, config map[string]interface{}) (BlobService, error)

	// GetResourceGroupService creates and returns a ResourceGroupService instance.
	// The service handles Azure Resource Group operations like creation, deletion,
	// and resource management.
	// config: Configuration map containing resource group parameters
	GetResourceGroupService(ctx context.Context, l log.Logger, config map[string]interface{}) (ResourceGroupService, error)

	// GetRBACService creates and returns an RBACService instance.
	// The service handles Azure Role-Based Access Control operations like
	// role assignments and permission management.
	// config: Configuration map containing RBAC parameters
	GetRBACService(ctx context.Context, l log.Logger, config map[string]interface{}) (RBACService, error)

	// GetAuthenticationService creates and returns an AuthenticationService instance.
	// The service handles Azure authentication operations like credential management
	// and token operations.
	// config: Configuration map containing authentication parameters
	GetAuthenticationService(ctx context.Context, l log.Logger, config map[string]interface{}) (AuthenticationService, error)

	// Configuration Management

	// Service Lifecycle Management

	// Initialize initializes the service container with the provided configuration.
	// This method should be called before using any service factory methods.
	// config: Global configuration for the service container
	Initialize(ctx context.Context, l log.Logger, config map[string]interface{}) error

	// Cleanup cleans up resources held by the service container.
	// This method should be called when the container is no longer needed.
	Cleanup(ctx context.Context, l log.Logger) error

	// Health checks the health of all services in the container.
	// Returns an error if any service is unhealthy.
	Health(ctx context.Context, l log.Logger) error

	// Reset resets the service container to its initial state.
	// This clears any cached services and forces them to be recreated on next access.
	Reset(ctx context.Context, l log.Logger) error

	// Service Registration (for advanced usage)

	// RegisterStorageAccountService registers a custom StorageAccountService implementation.
	// This allows for dependency injection of custom implementations.
	// service: The custom service implementation to register
	RegisterStorageAccountService(service StorageAccountService)

	// RegisterBlobService registers a custom BlobService implementation.
	// service: The custom service implementation to register
	RegisterBlobService(service BlobService)

	// RegisterResourceGroupService registers a custom ResourceGroupService implementation.
	// service: The custom service implementation to register
	RegisterResourceGroupService(service ResourceGroupService)

	// RegisterRBACService registers a custom RBACService implementation.
	// service: The custom service implementation to register
	RegisterRBACService(service RBACService)

	// RegisterAuthenticationService registers a custom AuthenticationService implementation.
	// service: The custom service implementation to register
	RegisterAuthenticationService(service AuthenticationService)

	// Introspection

	// GetRegisteredServices returns a list of all registered service types.
	// This is useful for debugging and service discovery.
	GetRegisteredServices() []string

	// HasService checks if a specific service type is registered.
	// serviceType: The service type to check ("storage", "blob", "resourcegroup", "rbac", "auth")
	HasService(serviceType string) bool

	// GetServiceInfo returns information about a specific service.
	// serviceType: The service type to get information about
	// Returns a map containing service metadata
	GetServiceInfo(serviceType string) (map[string]interface{}, error)
}

// ServiceContainerConfig represents configuration for the Azure service container.
// This configuration controls container behavior, caching, and service lifecycle management.
type ServiceContainerConfig struct {
	// LogLevel specifies the logging level for the service container.
	// Valid values: "debug", "info", "warn", "error"
	// Default: "info"
	LogLevel string

	// CacheTimeout specifies how long to cache service instances (in seconds).
	// After this timeout, cached instances are discarded and recreated.
	// Default: 300 (5 minutes)
	CacheTimeout int

	// MaxCacheSize specifies the maximum number of service instances to cache.
	// When the cache is full, least recently used instances are evicted.
	// Default: 100
	MaxCacheSize int

	// HealthCheckInterval specifies how often to perform health checks (in seconds).
	// Default: 60 (1 minute)
	HealthCheckInterval int

	// EnableCaching indicates whether to cache service instances.
	// When enabled, service instances are reused for the same configuration.
	// Default: true
	EnableCaching bool

	// EnableHealthChecks indicates whether to perform health checks on services.
	// When enabled, services are periodically checked for health.
	// Default: false
	EnableHealthChecks bool

	// EnableMetrics indicates whether to collect metrics about service usage.
	// When enabled, metrics are collected for service creation, usage, and errors.
	// Default: false
	EnableMetrics bool
}

const (
	defaultCacheTimeoutSeconds    = 300
	defaultMaxCacheSize           = 100
	defaultHealthCheckIntervalSec = 60
)

// DefaultServiceContainerConfig returns the default configuration for the service container.
func DefaultServiceContainerConfig() ServiceContainerConfig {
	return ServiceContainerConfig{
		EnableCaching:       true,
		CacheTimeout:        defaultCacheTimeoutSeconds,
		MaxCacheSize:        defaultMaxCacheSize,
		EnableHealthChecks:  false,
		HealthCheckInterval: defaultHealthCheckIntervalSec,
		EnableMetrics:       false,
		LogLevel:            "info",
	}
}
