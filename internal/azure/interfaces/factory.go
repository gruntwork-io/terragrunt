package interfaces

import (
	"context"
)

// ServiceFactory defines the interface for creating Azure service containers
type ServiceFactory interface {
	// CreateContainer creates a new AzureServiceContainer instance
	CreateContainer(ctx context.Context) AzureServiceContainer

	// Options returns the factory options
	Options() *FactoryOptions
}

// FactoryOptions defines options for configuring the service factory
type FactoryOptions struct {
	// EnableMocking enables the use of mock implementations
	EnableMocking bool

	// MockResponses contains predefined responses for mock services
	MockResponses map[string]interface{}

	// DefaultConfig contains default configuration values
	DefaultConfig map[string]interface{}

	// RetryConfig contains retry settings
	RetryConfig *RetryConfig
}

// RetryConfig defines retry behavior for Azure operations
type RetryConfig struct {
	// MaxRetries is the maximum number of retries
	MaxRetries int

	// RetryDelay is the delay between retries in seconds
	RetryDelay int

	// MaxDelay is the maximum delay between retries in seconds
	MaxDelay int

	// RetryableStatusCodes are HTTP status codes that should trigger a retry
	RetryableStatusCodes []int
}
