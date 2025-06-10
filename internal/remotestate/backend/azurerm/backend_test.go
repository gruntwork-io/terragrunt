// Package azurerm represents Azure storage backend for remote state
package azurerm

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackend(t *testing.T) {
	t.Parallel()

	backend := NewBackend()
	assert.NotNil(t, backend)
	assert.Equal(t, BackendName, backend.Name())
}

func TestBackendWithInvalidCredentials(t *testing.T) {
	t.Parallel()

	backend := NewBackend()
	ctx := context.Background()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	logger := log.Default()

	config := map[string]interface{}{
		"name": "valid-with-container-tags",
		"storage_account_name": "invalid-account",
		"storage_account_key":  "invalid-key",
		"container_name":      "test-container",
		"key":                "test/terraform.tfstate",
	}
	
	_, err = backend.NeedsBootstrap(ctx, logger, config, opts)
	assert.Error(t, err, "Should fail with invalid credentials")

	err = backend.Bootstrap(ctx, logger, config, opts)
	assert.Error(t, err, "Should fail with invalid credentials")

	err = backend.Delete(ctx, logger, config, opts)
	assert.Error(t, err, "Should fail with invalid credentials")
}

func TestBackendErrorHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewBackend()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	logger := log.Default()

	invalidConfig := map[string]interface{}{
		"storage_account_name": "invalid-account-name",
		"container_name":      "invalid$container",
		"key":                "test/terraform.tfstate",
	}

	_, err = backend.NeedsBootstrap(ctx, logger, invalidConfig, opts)
	assert.Error(t, err)

	err = backend.Bootstrap(ctx, logger, invalidConfig, opts)
	assert.Error(t, err)

	err = backend.Delete(ctx, logger, invalidConfig, opts)
	assert.Error(t, err)
}

func TestBackendConfigFiltering(t *testing.T) {
	t.Parallel()

	backend := NewBackend()
	config := map[string]interface{}{
		"storage_account_name": "testaccount",
		"container_name":      "test-container",
		"key":                "test/terraform.tfstate",
		"container_tags": map[string]string{
			"Environment": "Test",
		},
	}

	initArgs := backend.GetTFInitArgs(config)
	assert.NotContains(t, initArgs, "container_tags")
	assert.Contains(t, initArgs, "storage_account_name")
	assert.Contains(t, initArgs, "container_name")
	assert.Contains(t, initArgs, "key")
}

func TestAzureRMConfigValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string // Expected error message for failing tests
		checkConfig func(t *testing.T, config map[string]interface{})
	}{
		// Required Field Tests
		{
			name: "valid-minimal-config",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"storage_account_key":  "testkey",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
			expectError: false,
			checkConfig: func(t *testing.T, config map[string]interface{}) {
				tfConfig := Config(config).FilterOutTerragruntKeys()
				assert.Equal(t, config, tfConfig, "No Terragrunt-specific keys to filter")
			},
		},
		{
			name: "missing-storage-account",
			config: map[string]interface{}{
				"container_name": "test-container",
				"key":           "test/terraform.tfstate",
			},
			expectError: true,
			errorMsg:    "storage_account_name",
		},
		{
			name: "valid-with-container-tags",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"storage_account_key":  "testkey",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
					"Project":    "Terragrunt",
				},
			},
			expectError: false,
			checkConfig: func(t *testing.T, config map[string]interface{}) {
				tfConfig := Config(config).FilterOutTerragruntKeys()
				assert.NotContains(t, tfConfig, "container_tags")
				assert.Contains(t, tfConfig, "storage_account_name")
				assert.Contains(t, tfConfig, "container_name")
				assert.Contains(t, tfConfig, "key")
			},
		},
		{
			name: "valid-key-auth",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"storage_account_key":  "testkey",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
			expectError: false,
		},
		{
			name: "valid-sas-token-auth",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"sas_token":          "test-sas-token",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
			expectError: false,
		},
		{
			name: "valid-service-principal-auth",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"subscription_id":     "test-subscription",
				"tenant_id":          "test-tenant",
				"client_id":          "test-client",
				"client_secret":      "test-secret",
			},
			expectError: false,
		},
		// Error Cases
		{
			name: "invalid-container-tags-type",
			config: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags":     "invalid", // Should be map[string]string
			},
			expectError: true,
			errorMsg:    "container_tags",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extConfig, err := Config(tc.config).ExtendedAzureConfig()
			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, extConfig)

			if tc.checkConfig != nil {
				tc.checkConfig(t, tc.config)
			}

			// Verify Terragrunt-specific keys are filtered out of backend config
			backend := NewBackend()
			initArgs := backend.GetTFInitArgs(tc.config)
			assert.NotContains(t, initArgs, "container_tags")
		})
	}
}

func TestAuthenticationMethodValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		config        Config
		expectedError string
	}{
		{
			name: "valid key auth",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "testcontainer",
				"key":                "test/terraform.tfstate",
				"storage_account_key": "testkey",
			},
			expectedError: "",
		},
		{
			name: "valid sas token",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "testcontainer",
				"key":                "test/terraform.tfstate",
				"sas_token":          "testsas",
			},
			expectedError: "",
		},
		{
			name: "valid service principal",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "testcontainer",
				"key":                "test/terraform.tfstate",
				"subscription_id":     "sub-id",
				"tenant_id":          "tenant-id",
				"client_id":          "client-id",
				"client_secret":      "client-secret",
			},
			expectedError: "",
		},
		{
			name: "mixed auth key and sas",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "testcontainer",
				"key":                "test/terraform.tfstate",
				"storage_account_key": "testkey",
				"sas_token":          "testsas",
			},
			expectedError: "cannot specify multiple authentication methods",
		},
		{
			name: "incomplete service principal",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "testcontainer",
				"key":                "test/terraform.tfstate",
				"subscription_id":     "sub-id",
				"tenant_id":          "tenant-id",
				// Missing client_id and client_secret
			},
			expectedError: "missing required fields",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			extConfig, err := tc.config.ExtendedAzureConfig()
			if tc.expectedError == "" {
				require.NoError(t, err)
				require.NotNil(t, extConfig)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}
