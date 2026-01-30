//go:build azure

package azurerm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createBackendTestLogger creates a logger for testing purposes
func createBackendTestLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

// TestBackendInterfaceCompliance tests that the Azure backend implements the Backend interface correctly
func TestBackendInterfaceCompliance(t *testing.T) {
	t.Parallel()

	// Verify that azurerm.Backend implements the backend.Backend interface
	var _ backend.Backend = (*azurerm.Backend)(nil)

	// Test that we can create a backend
	b := azurerm.NewBackend(nil)
	require.NotNil(t, b)

	// Test that backend name is correct
	assert.Equal(t, "azurerm", b.Name())
}

// TestBackendGetTFInitArgs tests the GetTFInitArgs method
func TestBackendGetTFInitArgs(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	testCases := []struct {
		config   azurerm.Config
		expected map[string]interface{}
		name     string
	}{
		{
			name: "basic-config",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
			},
			expected: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
			},
		},
		{
			name: "config-with-auth",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expected: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"use_azuread_auth":     true,
			},
		},
		{
			name: "config-with-subscription",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"subscription_id":      "12345678-1234-1234-1234-123456789012",
			},
			expected: map[string]interface{}{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"subscription_id":      "12345678-1234-1234-1234-123456789012",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test GetTFInitArgs returns expected configuration
			args := azureBackend.GetTFInitArgs(backend.Config(tc.config))
			assert.NotNil(t, args)

			for key, expectedValue := range tc.expected {
				assert.Contains(t, args, key)
				assert.Equal(t, expectedValue, args[key])
			}
		})
	}
}

// TestBackendBootstrapValidation tests that Bootstrap validates configuration
func TestBackendBootstrapValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test invalid configurations
	invalidConfigs := []struct {
		name   string
		config azurerm.Config
		error  string
	}{
		{
			name:   "empty-config",
			config: azurerm.Config{},
			error:  "storage_account_name",
		},
		{
			name: "invalid-container-name",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "", // Empty container name should fail validation
				"key":                  "test.tfstate",
			},
			error: "container",
		},
		{
			name: "missing-key",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "valid-container",
			},
			error: "key",
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that Bootstrap properly validates configuration
			err := azureBackend.Bootstrap(ctx, l, backend.Config(tc.config), opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
		})
	}
}

// TestBackendNeedsBootstrapValidation tests that NeedsBootstrap validates configuration
func TestBackendNeedsBootstrapValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test invalid configurations
	invalidConfigs := []struct {
		name   string
		config azurerm.Config
		error  string
	}{
		{
			name:   "empty-config",
			config: azurerm.Config{},
			error:  "storage_account_name",
		},
		{
			name: "invalid-container-name",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "", // Empty container name should fail validation
				"key":                  "test.tfstate",
			},
			error: "container",
		},
		{
			name: "missing-key",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "valid-container",
			},
			error: "key",
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that NeedsBootstrap properly validates configuration
			_, err := azureBackend.NeedsBootstrap(ctx, l, backend.Config(tc.config), opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
		})
	}
}

// TestBackendContainerNameValidation tests container name validation rules
func TestBackendContainerNameValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		containerName string
		errorMsg      string
		expectError   bool
	}{
		{
			name:          "valid-container-name",
			containerName: "valid-container-name",
			expectError:   false,
		},
		{
			name:          "uppercase-letters",
			containerName: "INVALID-CONTAINER",
			expectError:   true,
			errorMsg:      "lowercase",
		},
		{
			name:          "too-short",
			containerName: "ab",
			expectError:   true,
			errorMsg:      "between 3 and 63 characters",
		},
		{
			name:          "too-long",
			containerName: strings.Repeat("a", 64),
			expectError:   true,
			errorMsg:      "between 3 and 63 characters",
		},
		{
			name:          "consecutive-hyphens",
			containerName: "invalid--container",
			expectError:   true,
			errorMsg:      "consecutive hyphens",
		},
		{
			name:          "starts-with-hyphen",
			containerName: "-invalid-container",
			expectError:   true,
			errorMsg:      "start and end",
		},
		{
			name:          "ends-with-hyphen",
			containerName: "invalid-container-",
			expectError:   true,
			errorMsg:      "start and end",
		},
		{
			name:          "empty-name",
			containerName: "",
			expectError:   true,
			errorMsg:      "missing required",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test container name validation through ValidateContainerName function
			err := azurerm.ValidateContainerName(tc.containerName)

			if tc.expectError {
				require.Error(t, err)

				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBackendPathValidation tests how the backend validates different state file paths
func TestBackendPathValidation(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	// Test different state file paths for validation
	testPaths := []struct {
		path  string
		valid bool
	}{
		{"terraform.tfstate", true},
		{"env/prod/terraform.tfstate", true},
		{"very/deep/nested/path/terraform.tfstate", true},
		{"path-with-hyphens/terraform.tfstate", true},
		{"path_with_underscores/terraform.tfstate", true},
		{"", false},                    // empty path
		{"../terraform.tfstate", true}, // relative paths are allowed
		{"./terraform.tfstate", true},  // current dir reference
	}

	for _, test := range testPaths {
		t.Run("path-"+strings.ReplaceAll(test.path, "/", "-"), func(t *testing.T) {
			t.Parallel()

			// Test GetTFInitArgs with different key paths
			config := azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  test.path,
			}

			args := azureBackend.GetTFInitArgs(backend.Config(config))
			assert.NotNil(t, args)

			if test.path != "" {
				assert.Equal(t, test.path, args["key"])
			}
		})
	}
}

// TestBackendIsVersionControlEnabled tests version control detection
func TestBackendIsVersionControlEnabled(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test with invalid config (should fail validation before making API calls)
	invalidConfig := azurerm.Config{
		"storage_account_name": "testaccount",
		"container_name":       "", // Empty container name should fail validation
		"key":                  "test.tfstate",
	}

	_, err = azureBackend.IsVersionControlEnabled(ctx, l, backend.Config(invalidConfig), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container")

	// Test with valid config structure but missing subscription (will fail auth validation)
	validConfig := azurerm.Config{
		"storage_account_name": "testaccount",
		"container_name":       "test-container",
		"key":                  "terraform.tfstate",
		"use_azuread_auth":     true,
	}

	_, err = azureBackend.IsVersionControlEnabled(ctx, l, backend.Config(validConfig), opts)
	// This should fail due to missing subscription_id, not due to API calls
	if err != nil {
		assert.True(t,
			strings.Contains(err.Error(), "subscription_id") ||
				strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "credential") ||
				strings.Contains(err.Error(), "auth") ||
				strings.Contains(err.Error(), "no such host"),
			"Error should be related to authentication/connectivity, got: %v", err)
	}
}

// TestBackendConfigurationOptions tests various configuration options are preserved
func TestBackendConfigurationOptions(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	// Test that all common Azure backend configuration options are preserved in GetTFInitArgs
	// Note: Some options like resource_group_name are filtered out as they're only used by Terragrunt
	fullConfig := azurerm.Config{
		"storage_account_name": "testaccount",
		"container_name":       "test-container",
		"key":                  "terraform.tfstate",
		"subscription_id":      "12345678-1234-1234-1234-123456789012",
		"tenant_id":            "87654321-4321-4321-4321-210987654321",
		"use_azuread_auth":     true,
		"use_msi":              false,
		"msi_endpoint":         "http://localhost:50342/oauth2/token",
		"use_oidc":             false,
		"oidc_request_token":   "test-token",
		"oidc_request_url":     "https://test.com/token",
		"client_id":            "test-client-id",
		"endpoint":             "https://test.blob.core.windows.net/",
		"environment":          "public",
		"metadata_host":        "https://management.azure.com/",
		"snapshot":             true,
	}

	args := azureBackend.GetTFInitArgs(backend.Config(fullConfig))
	assert.NotNil(t, args)

	// Verify that all configuration values are preserved
	for key, value := range fullConfig {
		assert.Contains(t, args, key, "Configuration key %s should be preserved", key)
		assert.Equal(t, value, args[key], "Configuration value for %s should be preserved", key)
	}
}

// TestBackendDeleteValidation tests that Delete validates configuration
func TestBackendDeleteValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test invalid configurations that should fail validation before making API calls
	invalidConfigs := []struct {
		name   string
		config azurerm.Config
		error  string
	}{
		{
			name:   "empty-config",
			config: azurerm.Config{},
			error:  "storage_account_name",
		},
		{
			name: "missing-container",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"key":                  "test.tfstate",
			},
			error: "container",
		},
		{
			name: "empty-container-name",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "",
				"key":                  "test.tfstate",
			},
			error: "container",
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that Delete properly validates configuration
			err := azureBackend.Delete(ctx, l, backend.Config(tc.config), opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
		})
	}
}

// TestBackendDeleteContainerValidation tests that DeleteContainer validates configuration
func TestBackendDeleteContainerValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test invalid configurations that should fail validation before making API calls
	invalidConfigs := []struct {
		name   string
		config azurerm.Config
		error  string
	}{
		{
			name:   "empty-config",
			config: azurerm.Config{},
			error:  "storage_account_name",
		},
		{
			name: "missing-container",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"key":                  "test.tfstate",
			},
			error: "container",
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that DeleteContainer properly validates configuration
			err := azureBackend.DeleteContainer(ctx, l, backend.Config(tc.config), opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
		})
	}
}

// TestBackendMigrateValidation tests that Migrate validates both source and destination configurations
func TestBackendMigrateValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test with invalid source config
	invalidSrcConfig := azurerm.Config{}
	validDstConfig := azurerm.Config{
		"storage_account_name": "destaccount",
		"container_name":       "dest-container",
		"key":                  "terraform.tfstate",
	}

	err = azureBackend.Migrate(ctx, l, backend.Config(invalidSrcConfig), backend.Config(validDstConfig), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_account_name")

	// Test with invalid destination config
	validSrcConfig := azurerm.Config{
		"storage_account_name": "srcaccount",
		"container_name":       "src-container",
		"key":                  "terraform.tfstate",
	}
	invalidDstConfig := azurerm.Config{}

	err = azureBackend.Migrate(ctx, l, backend.Config(validSrcConfig), backend.Config(invalidDstConfig), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_account_name")
}

// TestBackendConfigurationFiltering tests that Terragrunt-only configuration options are filtered correctly
func TestBackendConfigurationFiltering(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	// Test configuration with Terragrunt-only options that should be filtered out
	configWithTerragruntOptions := azurerm.Config{
		"storage_account_name":                 "testaccount",
		"container_name":                       "test-container",
		"key":                                  "terraform.tfstate",
		"create_storage_account_if_not_exists": true,        // Terragrunt-only
		"resource_group_name":                  "test-rg",   // Terragrunt-only
		"location":                             "East US",   // Terragrunt-only
		"account_kind":                         "StorageV2", // Terragrunt-only
		"storage_account_tags": map[string]string{ // Terragrunt-only
			"Environment": "test",
		},
		"subscription_id":  "12345678-1234-1234-1234-123456789012", // Should be kept
		"use_azuread_auth": true,                                   // Should be kept
	}

	args := azureBackend.GetTFInitArgs(backend.Config(configWithTerragruntOptions))
	assert.NotNil(t, args)

	// Verify that Terragrunt-only options are filtered out
	terragruntOnlyKeys := []string{
		"create_storage_account_if_not_exists",
		"resource_group_name",
		"location",
		"account_kind",
		"storage_account_tags",
	}

	for _, key := range terragruntOnlyKeys {
		assert.NotContains(t, args, key, "Terragrunt-only key %s should be filtered out", key)
	}

	// Verify that Terraform options are preserved
	terraformKeys := []string{
		"storage_account_name",
		"container_name",
		"key",
		"subscription_id",
		"use_azuread_auth",
	}

	for _, key := range terraformKeys {
		assert.Contains(t, args, key, "Terraform key %s should be preserved", key)
	}
}

// TestBackendConfigurationParsing tests configuration parsing edge cases
func TestBackendConfigurationParsing(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	testCases := []struct {
		config azurerm.Config
		name   string
		valid  bool
	}{
		{
			name: "nil-values",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"use_azuread_auth":     nil, // nil value should be handled
			},
			valid: true,
		},
		{
			name: "mixed-types",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"snapshot":             "true", // string instead of bool
				"use_azuread_auth":     false,
			},
			valid: true,
		},
		{
			name: "numeric-values",
			config: azurerm.Config{
				"storage_account_name":        "testaccount",
				"container_name":              "test-container",
				"key":                         "terraform.tfstate",
				"client_certificate_password": 12345, // numeric value
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that GetTFInitArgs handles different value types without panicking
			args := azureBackend.GetTFInitArgs(backend.Config(tc.config))
			assert.NotNil(t, args)

			// Verify basic required keys are present
			assert.Contains(t, args, "storage_account_name")
			assert.Contains(t, args, "container_name")
			assert.Contains(t, args, "key")
		})
	}
}

// TestBackendNameMethod tests the Name method returns correct backend name
func TestBackendNameMethod(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	// Test that Name returns the correct backend name
	assert.Equal(t, "azurerm", azureBackend.Name())

	// Test that the name is consistent across multiple calls
	name1 := azureBackend.Name()
	name2 := azureBackend.Name()
	assert.Equal(t, name1, name2)

	// Test that different backend instances return the same name
	anotherBackend := azurerm.NewBackend(nil)
	assert.Equal(t, azureBackend.Name(), anotherBackend.Name())
}

// TestBackendDeleteStorageAccountValidation tests that DeleteStorageAccount validates configuration
func TestBackendDeleteStorageAccountValidation(t *testing.T) {
	t.Parallel()

	l := createBackendTestLogger()
	azureBackend := azurerm.NewBackend(nil)
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.NonInteractive = true

	ctx := context.Background()

	// Test invalid configurations that should fail validation before making API calls
	invalidConfigs := []struct {
		name   string
		config azurerm.Config
		error  string
	}{
		{
			name:   "empty-config",
			config: azurerm.Config{},
			error:  "storage_account_name",
		},
		{
			name: "missing-storage-account",
			config: azurerm.Config{
				"container_name": "test-container",
				"key":            "test.tfstate",
			},
			error: "storage_account_name",
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that DeleteStorageAccount properly validates configuration
			err := azureBackend.DeleteStorageAccount(ctx, l, backend.Config(tc.config), opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
		})
	}
}

// TestBackendAdvancedConfigurationOptions tests advanced configuration scenarios
func TestBackendAdvancedConfigurationOptions(t *testing.T) {
	t.Parallel()

	azureBackend := azurerm.NewBackend(nil)

	testCases := []struct {
		config azurerm.Config
		name   string
	}{
		{
			name: "oidc-authentication",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"use_oidc":             true,
				"oidc_request_token":   "test-token",
				"oidc_request_url":     "https://test.com/token",
				"client_id":            "test-client-id",
			},
		},
		{
			name: "msi-authentication",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"use_msi":              true,
				"msi_endpoint":         "http://localhost:50342/oauth2/token",
			},
		},
		{
			name: "custom-environment",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"environment":          "usgovernment",
				"metadata_host":        "https://management.usgovcloudapi.net/",
			},
		},
		{
			name: "service-principal-auth",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "terraform.tfstate",
				"client_id":            "test-client-id",
				"client_secret":        "test-client-secret",
				"subscription_id":      "12345678-1234-1234-1234-123456789012",
				"tenant_id":            "87654321-4321-4321-4321-210987654321",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that GetTFInitArgs preserves all authentication-related configuration
			args := azureBackend.GetTFInitArgs(backend.Config(tc.config))
			assert.NotNil(t, args)

			// Verify that all provided configuration is preserved
			for key, expectedValue := range tc.config {
				assert.Contains(t, args, key, "Configuration key %s should be preserved", key)
				assert.Equal(t, expectedValue, args[key], "Configuration value for %s should be preserved", key)
			}
		})
	}
}
