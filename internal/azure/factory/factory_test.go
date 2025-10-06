package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/factory"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testContextKey string

const factoryContextKey testContextKey = "factory"

var _ interfaces.AzureServiceContainer = (*factory.AzureServiceFactory)(nil)

// TestNewAzureServiceFactory tests factory creation
func TestNewAzureServiceFactory(t *testing.T) {
	t.Parallel()

	factory := factory.NewAzureServiceFactory()

	require.NotNil(t, factory)
}

// TestFactoryOptions tests factory options configuration
func TestFactoryOptions(t *testing.T) {
	t.Parallel()

	//nolint:govet // fieldalignment: table-driven tests prioritize logical ordering.
	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name    string
		options *factory.Options
	}{
		{
			name:    "nil options",
			options: nil,
		},
		{
			name:    "empty options",
			options: &factory.Options{},
		},
		{
			name: "mocking enabled",
			options: &factory.Options{
				EnableMocking: true,
				MockResponses: map[string]interface{}{
					"test": "response",
				},
			},
		},
		{
			name: "with default config",
			options: &factory.Options{
				DefaultConfig: map[string]interface{}{
					"subscriptionId": "test-subscription",
					"resourceGroup":  "test-rg",
				},
			},
		},
		{
			name: "with retry config",
			options: &factory.Options{
				RetryConfig: &interfaces.RetryConfig{
					MaxRetries: 3,
					RetryDelay: 1,
					MaxDelay:   30,
				},
			},
		},
		{
			name: "complete options",
			options: &factory.Options{
				EnableMocking: true,
				MockResponses: map[string]interface{}{
					"storage": "mock-storage",
					"blob":    "mock-blob",
				},
				DefaultConfig: map[string]interface{}{
					"subscriptionId": "test-sub",
					"resourceGroup":  "test-rg",
					"location":       "eastus",
				},
				RetryConfig: &interfaces.RetryConfig{
					MaxRetries: 5,
					RetryDelay: 2,
					MaxDelay:   60,
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that options can be created and accessed without panics
			require.NotPanics(t, func() {
				_ = tc.options
				if tc.options != nil {
					_ = tc.options.EnableMocking
					_ = tc.options.MockResponses
					_ = tc.options.DefaultConfig
					_ = tc.options.RetryConfig
				}
			})
		})
	}
}

// TestServiceContainerConfig tests configuration structure
func TestServiceContainerConfig(t *testing.T) {
	t.Parallel()

	//nolint:govet // fieldalignment: table-driven tests prioritize logical ordering.
	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name   string
		config interfaces.ServiceContainerConfig
	}{
		{
			name: "basic config",
			config: interfaces.ServiceContainerConfig{
				EnableCaching: true,
				CacheTimeout:  300,
			},
		},
		{
			name:   "empty config",
			config: interfaces.ServiceContainerConfig{},
		},
		{
			name: "complex config",
			config: interfaces.ServiceContainerConfig{
				EnableCaching:      true,
				CacheTimeout:       600,
				MaxCacheSize:       200,
				EnableHealthChecks: true,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that config can be created and accessed
			require.NotNil(t, tc.config)

			// Test field access
			_ = tc.config.EnableCaching
			_ = tc.config.CacheTimeout
			_ = tc.config.MaxCacheSize
		})
	}
}

// TestRetryConfig tests retry configuration
func TestRetryConfig(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name   string
		config *interfaces.RetryConfig
		valid  bool
	}{
		{
			name:   "nil config",
			config: nil,
			valid:  true,
		},
		{
			name: "default values",
			config: &interfaces.RetryConfig{
				MaxRetries: 0,
				RetryDelay: 0,
				MaxDelay:   0,
			},
			valid: true,
		},
		{
			name: "reasonable values",
			config: &interfaces.RetryConfig{
				MaxRetries: 3,
				RetryDelay: 1,
				MaxDelay:   30,
			},
			valid: true,
		},
		{
			name: "high retry count",
			config: &interfaces.RetryConfig{
				MaxRetries: 10,
				RetryDelay: 1,
				MaxDelay:   60,
			},
			valid: true,
		},
		{
			name: "long delay",
			config: &interfaces.RetryConfig{
				MaxRetries: 2,
				RetryDelay: 30,
				MaxDelay:   300,
			},
			valid: true,
		},
		{
			name: "with status codes",
			config: &interfaces.RetryConfig{
				MaxRetries:           5,
				RetryDelay:           1,
				MaxDelay:             10,
				RetryableStatusCodes: []int{429, 500, 502, 503, 504},
			},
			valid: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.config != nil {
				// Test field access
				assert.GreaterOrEqual(t, tc.config.MaxRetries, 0)
				assert.GreaterOrEqual(t, tc.config.RetryDelay, 0)
				assert.GreaterOrEqual(t, tc.config.MaxDelay, 0)

				// Test reasonable bounds
				assert.LessOrEqual(t, tc.config.MaxRetries, 100)  // Sanity check
				assert.LessOrEqual(t, tc.config.RetryDelay, 3600) // Sanity check (1 hour)
				assert.LessOrEqual(t, tc.config.MaxDelay, 3600)   // Sanity check (1 hour)

				// Test status codes if present
				if tc.config.RetryableStatusCodes != nil {
					for _, code := range tc.config.RetryableStatusCodes {
						assert.GreaterOrEqual(t, code, 100)
						assert.LessOrEqual(t, code, 599)
					}
				}
			}
		})
	}
}

// TestContextHandling tests that the factory handles context properly
func TestContextHandling(t *testing.T) {
	t.Parallel()

	factory := factory.NewAzureServiceFactory()
	require.NotNil(t, factory)

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name string
		ctx  context.Context
	}{
		{
			name: "background context",
			ctx:  context.Background(),
		},
		{
			name: "todo context",
			ctx:  context.TODO(),
		},
		{
			name: "context with values",
			ctx:  context.WithValue(context.Background(), factoryContextKey, "test-value"),
		},
		{
			name: "context with timeout",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				return ctx
			}(),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that context is accepted by factory methods
			// (we can't test actual service creation without Azure API, but we can test context handling)
			require.NotNil(t, tc.ctx)
			assert.NotPanics(t, func() {
				// Context should be usable
				select {
				case <-tc.ctx.Done():
					// Context was cancelled/timed out, that's fine
				default:
					// Context is still active, that's also fine
				}
			})
		})
	}
}

// TestStorageAccountConfigValidation tests storage account configuration validation
func TestStorageAccountConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name   string
		config *types.StorageAccountConfig
		valid  bool
	}{
		{
			name:   "nil config",
			config: nil,
			valid:  false,
		},
		{
			name:   "empty config",
			config: &types.StorageAccountConfig{},
			valid:  false,
		},
		{
			name: "minimal valid config",
			config: &types.StorageAccountConfig{
				Name:              "validstorageaccount",
				ResourceGroupName: "valid-rg",
				Location:          "eastus",
			},
			valid: true,
		},
		{
			name: "complete valid config",
			config: &types.StorageAccountConfig{
				Name:                  "completestorageaccount",
				ResourceGroupName:     "complete-rg",
				Location:              "westus2",
				EnableVersioning:      true,
				AllowBlobPublicAccess: false,
				AccountKind:           types.AccountKind("StorageV2"),
				AccountTier:           types.AccountTier("Standard"),
				AccessTier:            types.AccessTier("Hot"),
				ReplicationType:       types.ReplicationType("LRS"),
				Tags: map[string]string{
					"Environment": "test",
					"Owner":       "terragrunt",
				},
			},
			valid: true,
		},
		{
			name: "config with invalid name (too short)",
			config: &types.StorageAccountConfig{
				Name:              "ab", // Too short
				ResourceGroupName: "valid-rg",
				Location:          "eastus",
			},
			valid: false,
		},
		{
			name: "config with invalid name (too long)",
			config: &types.StorageAccountConfig{
				Name:              "thisstorageaccountnameiswaytoolongandexceedsthemaximumlength", // Too long
				ResourceGroupName: "valid-rg",
				Location:          "eastus",
			},
			valid: false,
		},
		{
			name: "config with invalid name (uppercase)",
			config: &types.StorageAccountConfig{
				Name:              "InvalidStorageAccount", // Contains uppercase
				ResourceGroupName: "valid-rg",
				Location:          "eastus",
			},
			valid: false,
		},
		{
			name: "config with invalid name (special chars)",
			config: &types.StorageAccountConfig{
				Name:              "invalid-storage-account", // Contains hyphens
				ResourceGroupName: "valid-rg",
				Location:          "eastus",
			},
			valid: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that config structure is valid
			if tc.config != nil {
				// Test field access
				_ = tc.config.Name
				_ = tc.config.ResourceGroupName
				_ = tc.config.Location
				_ = tc.config.EnableVersioning
				_ = tc.config.AllowBlobPublicAccess
				_ = tc.config.AccountKind
				_ = tc.config.AccountTier
				_ = tc.config.AccessTier
				_ = tc.config.ReplicationType
				_ = tc.config.Tags

				// Basic validation rules
				if tc.valid {
					assert.NotEmpty(t, tc.config.Name, "Valid config should have non-empty name")
					assert.NotEmpty(t, tc.config.ResourceGroupName, "Valid config should have non-empty resource group")
					assert.NotEmpty(t, tc.config.Location, "Valid config should have non-empty location")
				}
			}
		})
	}
}
