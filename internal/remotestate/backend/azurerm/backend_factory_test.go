//nolint:testpackage // Factory tests require access to internal constructors.
package azurerm

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnhancedBackendConfigCreation tests creation of backend with enhanced configuration
func TestEnhancedBackendConfigCreation(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name                  string
		config                *BackendConfig
		expectEnhancedFactory bool
	}{
		{
			name:                  "nil config gets defaults",
			config:                nil,
			expectEnhancedFactory: true,
		},
		{
			name:                  "empty config gets defaults",
			config:                &BackendConfig{},
			expectEnhancedFactory: true,
		},
		{
			name: "custom retry config",
			config: &BackendConfig{
				RetryConfig: &interfaces.RetryConfig{
					MaxRetries: 5,
					RetryDelay: 2,
					MaxDelay:   60,
				},
			},
			expectEnhancedFactory: true,
		},
		{
			name: "custom telemetry settings",
			config: &BackendConfig{
				TelemetrySettings: &TelemetrySettings{
					EnableDetailedMetrics: false,
					EnableErrorTracking:   true,
					MetricsBufferSize:     500,
					FlushInterval:         60,
				},
			},
			expectEnhancedFactory: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := NewBackend(tc.config)
			require.NotNil(t, backend)
			require.NotNil(t, backend.serviceFactory)
			require.NotNil(t, backend.serviceContainer)

			if tc.expectEnhancedFactory {
				_, isEnhanced := backend.serviceFactory.(*enhancedServiceFactory)
				assert.True(t, isEnhanced, "Expected enhanced service factory")
			}
		})
	}
}

// TestBackendFromRemoteStateConfig tests creation from remote state config
func TestBackendFromRemoteStateConfig(t *testing.T) {
	t.Parallel()

	config := backend.Config{
		"storage_account_name": "testaccount",
		"container_name":       "testcontainer",
		"key":                  "terraform.tfstate",
		"use_azuread_auth":     true,
		"subscription_id":      "test-subscription-id",
	}

	opts := &options.TerragruntOptions{
		RetryMaxAttempts: 5,
	}

	backend, err := NewBackendFromRemoteStateConfig(config, opts)
	require.NoError(t, err)
	require.NotNil(t, backend)

	// Verify that the retry configuration was extracted
	factoryConfig := backend.GetFactoryConfiguration()
	require.NotEmpty(t, factoryConfig)

	// Check auth configuration
	if authConfig, exists := factoryConfig["auth"]; exists {
		if authMap, ok := authConfig.(map[string]interface{}); ok {
			assert.Equal(t, "azuread", authMap["preferred_auth_method"])
			assert.Equal(t, true, authMap["enable_auth_caching"])
		}
	}
}

// TestFactoryConfigurationUpdates tests runtime configuration updates
func TestFactoryConfigurationUpdates(t *testing.T) {
	t.Parallel()

	backend := NewBackend(&BackendConfig{})
	require.NotNil(t, backend)

	// Get initial configuration
	initialConfig := backend.GetFactoryConfiguration()
	require.NotEmpty(t, initialConfig)

	// Update telemetry settings
	updates := map[string]interface{}{
		"telemetry": map[string]interface{}{
			"enable_detailed_metrics": false,
			"metrics_buffer_size":     2000,
		},
		"auth": map[string]interface{}{
			"preferred_auth_method": "msi",
			"enable_auth_caching":   false,
		},
	}

	err := backend.UpdateFactoryConfiguration(updates)
	require.NoError(t, err)

	// Verify updates were applied
	updatedConfig := backend.GetFactoryConfiguration()
	if telemetryConfig, exists := updatedConfig["telemetry"]; exists {
		if telemetryMap, ok := telemetryConfig.(map[string]interface{}); ok {
			assert.Equal(t, false, telemetryMap["enable_detailed_metrics"])
			assert.Equal(t, 2000, telemetryMap["metrics_buffer_size"])
		}
	}

	if authConfig, exists := updatedConfig["auth"]; exists {
		if authMap, ok := authConfig.(map[string]interface{}); ok {
			assert.Equal(t, "msi", authMap["preferred_auth_method"])
			assert.Equal(t, false, authMap["enable_auth_caching"])
		}
	}
}

// TestExtractPreferredAuthMethod tests auth method detection from config
func TestExtractPreferredAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         backend.Config
		expectedMethod string
	}{
		{
			name: "azure ad auth",
			config: backend.Config{
				"use_azuread_auth": true,
			},
			expectedMethod: "azuread",
		},
		{
			name: "msi auth",
			config: backend.Config{
				"use_msi": true,
			},
			expectedMethod: "msi",
		},
		{
			name: "service principal",
			config: backend.Config{
				"client_id": "test-client-id",
			},
			expectedMethod: "service_principal",
		},
		{
			name: "sas token",
			config: backend.Config{
				"sas_token": "test-sas-token",
			},
			expectedMethod: "sas_token",
		},
		{
			name: "access key",
			config: backend.Config{
				"access_key": "test-access-key",
			},
			expectedMethod: "access_key",
		},
		{
			name:           "default to azuread",
			config:         backend.Config{},
			expectedMethod: "azuread",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			method := extractPreferredAuthMethod(tc.config)
			assert.Equal(t, tc.expectedMethod, method)
		})
	}
}
