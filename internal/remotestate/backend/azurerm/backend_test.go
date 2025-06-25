// Package azurerm represents Azure storage backend for remote state
package azurerm_test

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestNewBackend(t *testing.T) {
	t.Parallel()

	b := azurerm.NewBackend()
	require.NotNil(t, b)
	assert.IsType(t, &azurerm.Backend{}, b)
}

func TestBackendBootstrapInvalidConfig(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	testCases := []struct {
		config      backend.Config // map pointer (8 bytes) - place first for alignment
		name        string         // string (16 bytes) - after pointer type
		expectError bool           // bool (1 byte) - at end for padding
	}{
		{
			name: "missing-storage-account",
			config: backend.Config{
				"container_name": "test-container",
				"key":            "test/terraform.tfstate",
			},
			expectError: true,
		},
		{
			name: "missing-container",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"key":                  "test/terraform.tfstate",
			},
			expectError: true,
		},
		{
			name: "missing-key",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
			},
			expectError: true,
		},
	}

	b := azurerm.NewBackend()
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := b.NeedsBootstrap(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestDeleteStorageAccount(t *testing.T) {
	t.Parallel()

	// Create a logger for testing
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Do not make actual API calls for this test
	opts.NonInteractive = true

	// Create a backend instance
	b := azurerm.NewBackend()

	// Test with missing resource group name
	t.Run("MissingResourceGroupName", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "teststorageaccount",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource_group_name is required")
	})

	// Test with missing subscription ID
	t.Run("MissingSubscriptionID", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "teststorageaccount",
			"resource_group_name":  "test-rg",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription_id is required")
	})

	// Test with valid configuration in interactive mode
	t.Run("ValidConfiguration_Interactive", func(t *testing.T) {
		t.Parallel()

		interactiveOpts := *opts
		interactiveOpts.NonInteractive = false

		config := backend.Config{
			"storage_account_name": "teststorageaccount",
			"resource_group_name":  "test-rg",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// In interactive mode with no TTY, we'll get some kind of error
		// when trying to read from stdin during the prompt
		err := b.DeleteStorageAccount(t.Context(), l, config, &interactiveOpts)
		require.Error(t, err)
		// The specific error can vary between environments (could be "EOF", "not a terminal", etc.)
		// So we just check that we get an error, but don't check the specific message
		require.Error(t, err)
	})

	// Test with valid configuration in non-interactive mode
	t.Run("ValidConfiguration_NonInteractive", func(t *testing.T) {
		t.Parallel()

		nonInteractiveOpts := *opts
		nonInteractiveOpts.NonInteractive = true

		config := backend.Config{
			"storage_account_name": "teststorageaccount",
			"resource_group_name":  "test-rg",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// In non-interactive mode, we should get an error saying we can't delete without confirmation
		err := b.DeleteStorageAccount(t.Context(), l, config, &nonInteractiveOpts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-interactive")
		assert.Contains(t, err.Error(), "user confirmation is required")
	})
}

// TestAzureBackendBootstrapScenarios tests different Azure bootstrap scenarios
func TestAzureBackendBootstrapScenarios(t *testing.T) {
	t.Parallel()

	// Create logger for testing
	l := createLogger()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	// Create a unique suffix for storage account names
	uniqueSuffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	uniqueSuffix = uniqueSuffix[len(uniqueSuffix)-8:] // Last 8 digits

	// Create a backend instance
	b := azurerm.NewBackend()

	// Test cases for various bootstrap scenarios
	testCases := []struct {
		// Map type first
		config backend.Config
		// String fields
		name     string
		errorMsg string
		// Boolean field last
		expectError bool
	}{
		{
			name: "bootstrap-with-storage-account-creation",
			config: backend.Config{
				"storage_account_name":                 "tgtestsa" + uniqueSuffix + "2",
				"container_name":                       "tfstate",
				"key":                                  "test/terraform.tfstate",
				"subscription_id":                      "00000000-0000-0000-0000-000000000000", // Required
				"resource_group_name":                  "satestrg" + uniqueSuffix,              // Required
				"location":                             "eastus",                               // Required
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
			},
			expectError: true,                               // Will fail in unit test because it actually tries to connect to Azure
			errorMsg:    "does not exist in resource group", // Actual error message when trying to check storage account
		},
		{
			name: "missing-location-with-create",
			config: backend.Config{
				"storage_account_name":                 fmt.Sprintf("tgtestsa%s3", uniqueSuffix),
				"container_name":                       "tfstate",
				"key":                                  "test/terraform.tfstate",
				"subscription_id":                      "00000000-0000-0000-0000-000000000000",
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
				"resource_group_name":                  "satestrg" + uniqueSuffix,
			},
			expectError: true,
			errorMsg:    "location is required for storage account creation", // Missing required location field
		},
		{
			name: "missing-subscription-id-with-create",
			config: backend.Config{
				"storage_account_name":                 "tgtestsa" + uniqueSuffix + "4",
				"container_name":                       "tfstate",
				"key":                                  "test/terraform.tfstate",
				"location":                             "eastus",
				"resource_group_name":                  "satestrg" + uniqueSuffix,
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
			},
			expectError: true,
			errorMsg:    "subscription_id is required for storage account creation", // Missing required subscription_id field
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run the bootstrap function
			err := b.Bootstrap(t.Context(), l, tc.config, opts)

			// Check if we get expected results
			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestStorageAccountCreationConfig tests the configuration for storage account creation
func TestStorageAccountCreationConfig(t *testing.T) {
	t.Parallel()

	// Test with basic storage account creation configuration
	t.Run("BasicStorageAccountConfig", func(t *testing.T) {
		t.Parallel()
		config := backend.Config{
			"storage_account_name":                 "mystorageaccount",
			"container_name":                       "terraform-state",
			"key":                                  "terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "my-resource-group",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
		}

		// Parse the extended config
		extConfig, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Verify storage account creation configuration
		assert.Equal(t, "mystorageaccount", extConfig.RemoteStateConfigAzurerm.StorageAccountName)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", extConfig.RemoteStateConfigAzurerm.SubscriptionID)
		assert.Equal(t, "my-resource-group", extConfig.StorageAccountConfig.ResourceGroupName)
		assert.Equal(t, "eastus", extConfig.StorageAccountConfig.Location)
		assert.True(t, extConfig.StorageAccountConfig.CreateStorageAccountIfNotExists)
		assert.True(t, extConfig.StorageAccountConfig.EnableVersioning)       // Default value
		assert.False(t, extConfig.StorageAccountConfig.AllowBlobPublicAccess) // Default value
	})

	// Test with complete storage account configuration
	t.Run("CompleteStorageAccountConfig", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name":                 "mystorageaccount",
			"container_name":                       "terraform-state",
			"key":                                  "terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "my-resource-group",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"enable_versioning":                    false, // Explicitly disable versioning
			"allow_blob_public_access":             true,  // Explicitly enable public access
			"enable_hierarchical_namespace":        true,
			"account_kind":                         "BlobStorage",
			"account_tier":                         "Premium",
			"access_tier":                          "Cool",
			"replication_type":                     "GRS",
			"storage_account_tags": map[string]string{
				"Environment": "Dev",
				"Owner":       "Terragrunt",
			},
			"use_azuread_auth": true,
		}

		// Parse the extended config
		extConfig, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Verify storage account creation configuration
		assert.Equal(t, "mystorageaccount", extConfig.RemoteStateConfigAzurerm.StorageAccountName)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", extConfig.RemoteStateConfigAzurerm.SubscriptionID)
		assert.Equal(t, "my-resource-group", extConfig.StorageAccountConfig.ResourceGroupName)
		assert.Equal(t, "eastus", extConfig.StorageAccountConfig.Location)
		assert.True(t, extConfig.StorageAccountConfig.CreateStorageAccountIfNotExists)
		assert.False(t, extConfig.StorageAccountConfig.EnableVersioning)     // Explicitly set
		assert.True(t, extConfig.StorageAccountConfig.AllowBlobPublicAccess) // Explicitly set
		assert.True(t, extConfig.StorageAccountConfig.EnableHierarchicalNS)
		assert.Equal(t, "BlobStorage", extConfig.StorageAccountConfig.AccountKind)
		assert.Equal(t, "Premium", extConfig.StorageAccountConfig.AccountTier)
		assert.Equal(t, "Cool", extConfig.StorageAccountConfig.AccessTier)
		assert.Equal(t, "GRS", extConfig.StorageAccountConfig.ReplicationType)
		assert.Equal(t, map[string]string{
			"Environment": "Dev",
			"Owner":       "Terragrunt",
		}, extConfig.StorageAccountConfig.StorageAccountTags)
	})

	// Test missing required fields for storage account creation
	t.Run("MissingRequiredFields", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name":                 "testterragrunt",
			"container_name":                       "terraform-state",
			"key":                                  "terraform.tfstate",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
		}

		// Parse the extended config - just verify parsing succeeds
		_, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Config parsing succeeds but bootstrap would fail because subscription_id and location are required
		l := createLogger()
		opts, _ := options.NewTerragruntOptionsForTest("")
		b := azurerm.NewBackend()
		err = b.Bootstrap(t.Context(), l, config, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription_id", "Error should mention missing subscription_id")

		// Verify the error is the expected custom type
		var missingSubError azurerm.MissingSubscriptionIDError
		require.ErrorAs(t, err, &missingSubError, "Error should be MissingSubscriptionIDError type")
	})
}

// TestAzureAuthenticationOptions tests the available authentication options
func TestAzureAuthenticationOptions(t *testing.T) {
	t.Parallel()

	// Test Azure AD authentication
	t.Run("AzureADAuth", func(t *testing.T) {
		t.Parallel()
		config := backend.Config{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Parse the extended config
		extConfig, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Verify Azure AD auth configuration
		assert.True(t, extConfig.RemoteStateConfigAzurerm.UseAzureADAuth)
		assert.False(t, extConfig.RemoteStateConfigAzurerm.UseMsi)
	})

	// Test Managed Identity authentication
	t.Run("ManagedIdentityAuth", func(t *testing.T) {
		t.Parallel()
		config := backend.Config{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"use_msi":              true,
			"use_azuread_auth":     false, // Explicitly disable Azure AD auth when using MSI
		}

		// Parse the extended config
		extConfig, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Verify MSI auth configuration
		assert.True(t, extConfig.RemoteStateConfigAzurerm.UseMsi)
	})

	// Test service principal authentication
	t.Run("ServicePrincipalAuth", func(t *testing.T) {
		t.Parallel()
		config := backend.Config{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"tenant_id":            "00000000-0000-0000-0000-000000000000",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"client_id":            "00000000-0000-0000-0000-000000000000",
			"client_secret":        "supersecret",
			"use_azuread_auth":     false, // Disable default Azure AD auth since we're using service principal
		}

		// Parse the extended config
		extConfig, err := azurerm.Config(config).ExtendedAzureConfig()
		require.NoError(t, err)

		// Verify service principal auth configuration
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", extConfig.RemoteStateConfigAzurerm.TenantID)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", extConfig.RemoteStateConfigAzurerm.SubscriptionID)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", extConfig.RemoteStateConfigAzurerm.ClientID)
		assert.Equal(t, "supersecret", extConfig.RemoteStateConfigAzurerm.ClientSecret)
	})
}

// TestBlobServiceClientCreationError tests the error handling path when blob service client creation fails
func TestBlobServiceClientCreationError(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Setup test cases with configurations that will cause blob service client creation to fail
	testCases := []struct {
		name           string
		config         backend.Config
		expectedErrMsg string // Expected error message substring
	}{
		{
			name: "empty-storage-account-name",
			config: backend.Config{
				"storage_account_name": "", // Empty storage account name
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectedErrMsg: "missing required Azure remote state configuration storage_account_name",
		},
		{
			name: "unsupported-auth-method",
			config: backend.Config{
				"storage_account_name": "teststorageaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     false, // Azure AD auth is now required
				"use_msi":              false,
			},
			expectedErrMsg: "authentication failed", // Actual error message from the Azure API
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a new backend instance for testing
			b := azurerm.NewBackend()

			// Call Bootstrap and expect an error
			err := b.Bootstrap(t.Context(), l, tc.config, opts)

			// Verify an error was returned and it contains the expected message
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrMsg)
		})
	}
}

// TestContainerCreationError tests the error handling path when container creation fails
func TestContainerCreationError(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Setup test cases with configurations that will cause container creation to fail
	testCases := []struct {
		name           string
		config         backend.Config
		expectedErrMsg string // Expected error message substring
	}{
		{
			name: "invalid-container-name",
			config: backend.Config{
				"storage_account_name": "teststorageaccount",
				"container_name":       "Invalid-Container-Name-With-Upper-Case", // Invalid container name with uppercase
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectedErrMsg: "container name can only contain lowercase letters, numbers, and hyphens",
		},
		{
			name: "empty-container-name",
			config: backend.Config{
				"storage_account_name": "teststorageaccount",
				"container_name":       "", // Empty container name
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectedErrMsg: "missing required Azure remote state configuration container_name",
		},
		{
			name: "container-name-too-short",
			config: backend.Config{
				"storage_account_name": "teststorageaccount",
				"container_name":       "t", // Container name too short (< 3 characters)
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectedErrMsg: "container name must be between 3 and 63 characters",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Skip actual test execution if we're in a normal test environment, since these tests
			// would try to make actual Azure API calls and we want them to fail safely
			// Only run these tests with a custom environment variable in CI or special testing scenarios
			if os.Getenv("TG_TEST_AZURE_ERROR_PATHS") != "true" {
				t.Skip("Skipping container creation error tests. Set TG_TEST_AZURE_ERROR_PATHS=true to enable")
			}

			// Create a new backend instance for testing
			b := azurerm.NewBackend()

			// Call Bootstrap and expect an error
			err := b.Bootstrap(t.Context(), l, tc.config, opts)

			// Verify an error was returned and it contains the expected message
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrMsg)
		})
	}
}

// TestStorageAccountConfigOptions tests the handling of various storage account configuration options
func TestStorageAccountConfigOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		// Maps first (larger alignment requirements)
		config           map[string]interface{}
		expectedSAConfig map[string]interface{}
		// String fields
		name string
		// Boolean fields last
		expectError        bool
		skipExtendedChecks bool // Skip extended checks for standard configs
	}{
		{
			name: "basic-config",
			config: map[string]interface{}{
				"storage_account_name": "teststorageaccount",
				"container_name":       "testcontainer",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectedSAConfig: map[string]interface{}{
				"EnableVersioning":      true,  // Default value
				"AllowBlobPublicAccess": false, // Default value
				"EnableHierarchicalNS":  false, // Default value
				"AccountKind":           "",    // Empty means default
				"AccountTier":           "",    // Empty means default
				"AccessTier":            "",    // Empty means default
				"ReplicationType":       "",    // Empty means default
			},
			skipExtendedChecks: true, // This is a basic config without extended options
		},
		{
			name: "complete-storage-config",
			config: map[string]interface{}{
				"storage_account_name":                 "teststorageaccount",
				"container_name":                       "testcontainer",
				"key":                                  "test/terraform.tfstate",
				"use_azuread_auth":                     true,
				"location":                             "eastus",
				"resource_group_name":                  "test-resource-group",
				"enable_versioning":                    false,
				"allow_blob_public_access":             true,
				"enable_hierarchical_namespace":        true,
				"account_kind":                         "BlockBlobStorage",
				"account_tier":                         "Premium",
				"access_tier":                          "Cool",
				"replication_type":                     "ZRS",
				"create_storage_account_if_not_exists": true,
			},
			expectedSAConfig: map[string]interface{}{
				"EnableVersioning":      false,
				"AllowBlobPublicAccess": true,
				"EnableHierarchicalNS":  true,
				"AccountKind":           "BlockBlobStorage",
				"AccountTier":           "Premium",
				"AccessTier":            "Cool",
				"ReplicationType":       "ZRS",
				"Location":              "eastus",
				"ResourceGroupName":     "test-resource-group",
			},
		},
		{
			name: "legacy-blob-public-access-naming",
			config: map[string]interface{}{
				"storage_account_name":       "teststorageaccount",
				"container_name":             "testcontainer",
				"key":                        "test/terraform.tfstate",
				"use_azuread_auth":           true,
				"disable_blob_public_access": true, // Legacy naming
			},
			expectedSAConfig: map[string]interface{}{
				"AllowBlobPublicAccess": false, // Should be set to false when disable_blob_public_access is true
			},
		},
		{
			name: "alternative-replication-naming",
			config: map[string]interface{}{
				"storage_account_name": "teststorageaccount",
				"container_name":       "testcontainer",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				"replication_type":     "GZRS", // Using the correct field name
			},
			expectedSAConfig: map[string]interface{}{
				"ReplicationType": "GZRS", // Should be set correctly
			},
		},
		{
			name: "storage-account-tags",
			config: map[string]interface{}{
				"storage_account_name": "teststorageaccount",
				"container_name":       "testcontainer",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				"storage_account_tags": map[string]string{
					"Environment": "Test",
					"Owner":       "Terragrunt",
					"Project":     "Azure Backend",
				},
			},
			expectedSAConfig: map[string]interface{}{
				"StorageAccountTags": map[string]string{
					"Environment": "Test",
					"Owner":       "Terragrunt",
					"Project":     "Azure Backend",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Convert to backend.Config
			backendConfig := backend.Config{}
			for k, v := range tc.config {
				backendConfig[k] = v
			}

			// Parse extended Azure config
			azureCfg, err := azurerm.Config(backendConfig).ExtendedAzureConfig()

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, azureCfg)

			// Check if basic options are set correctly
			if v, exists := tc.expectedSAConfig["EnableVersioning"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.EnableVersioning)
			}

			if v, exists := tc.expectedSAConfig["AllowBlobPublicAccess"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.AllowBlobPublicAccess)
			}

			if v, exists := tc.expectedSAConfig["EnableHierarchicalNS"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.EnableHierarchicalNS)
			}

			// Check storage account kind and tier if specified
			if v, exists := tc.expectedSAConfig["AccountKind"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.AccountKind)
			}

			if v, exists := tc.expectedSAConfig["AccountTier"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.AccountTier)
			}

			if v, exists := tc.expectedSAConfig["AccessTier"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.AccessTier)
			}

			if v, exists := tc.expectedSAConfig["ReplicationType"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.ReplicationType)
			}

			if v, exists := tc.expectedSAConfig["Location"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.Location)
			}

			if v, exists := tc.expectedSAConfig["ResourceGroupName"]; exists && !tc.skipExtendedChecks {
				assert.Equal(t, v, azureCfg.StorageAccountConfig.ResourceGroupName)
			}

			// Check storage account tags if specified
			if tags, exists := tc.expectedSAConfig["StorageAccountTags"].(map[string]string); exists && !tc.skipExtendedChecks {
				require.Len(t, azureCfg.StorageAccountConfig.StorageAccountTags, len(tags))

				for k, v := range tags {
					actualValue, ok := azureCfg.StorageAccountConfig.StorageAccountTags[k]
					assert.True(t, ok, "Tag %s not found", k)
					assert.Equal(t, v, actualValue, "Tag %s value mismatch", k)
				}
			}
		})
	}
}

// TestContainerNameValidation tests the container name validation function without Azure operations
func TestContainerNameValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		containerName  string
		expectedErrMsg string
		expectError    bool
	}{
		{
			name:          "valid-container-name",
			containerName: "valid-container-name",
			expectError:   false,
		},
		{
			name:          "valid-short-name",
			containerName: "abc",
			expectError:   false,
		},
		{
			name:          "valid-with-numbers",
			containerName: "container123",
			expectError:   false,
		},
		{
			name:           "empty-container-name",
			containerName:  "",
			expectError:    true,
			expectedErrMsg: "missing required Azure remote state configuration container_name",
		},
		{
			name:           "container-name-too-short",
			containerName:  "ab",
			expectError:    true,
			expectedErrMsg: "container name must be between 3 and 63 characters",
		},
		{
			name:           "container-name-too-long",
			containerName:  strings.Repeat("a", 64),
			expectError:    true,
			expectedErrMsg: "container name must be between 3 and 63 characters",
		},
		{
			name:           "invalid-uppercase",
			containerName:  "Invalid-Container-Name",
			expectError:    true,
			expectedErrMsg: "container name can only contain lowercase letters, numbers, and hyphens",
		},
		{
			name:           "invalid-special-chars",
			containerName:  "container_name",
			expectError:    true,
			expectedErrMsg: "container name can only contain lowercase letters, numbers, and hyphens",
		},
		{
			name:           "invalid-starts-with-hyphen",
			containerName:  "-container",
			expectError:    true,
			expectedErrMsg: "container name must start and end with a letter or number",
		},
		{
			name:           "invalid-ends-with-hyphen",
			containerName:  "container-",
			expectError:    true,
			expectedErrMsg: "container name must start and end with a letter or number",
		},
		{
			name:           "invalid-consecutive-hyphens",
			containerName:  "container--name",
			expectError:    true,
			expectedErrMsg: "container name cannot contain consecutive hyphens",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := azurerm.ValidateContainerName(tc.containerName)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCustomErrorTypes tests the custom error types and their unwrapping functionality
func TestCustomErrorTypes(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	t.Run("MissingSubscriptionIDError", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name":                 "mystorageaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"resource_group_name":                  "my-resource-group",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// subscription_id is missing
		}

		b := azurerm.NewBackend()
		err := b.Bootstrap(t.Context(), l, config, opts)

		require.Error(t, err)

		// Check that the error is of the expected custom type
		var missingSubError azurerm.MissingSubscriptionIDError
		require.ErrorAs(t, err, &missingSubError, "Error should be MissingSubscriptionIDError type")

		// Check the error message
		assert.Contains(t, err.Error(), "subscription_id is required for storage account creation")
	})

	t.Run("MissingLocationError", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name":                 "mystorageaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "my-resource-group",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// location is missing
		}

		b := azurerm.NewBackend()
		err := b.Bootstrap(t.Context(), l, config, opts)

		require.Error(t, err)

		// Check that the error is of the expected custom type
		var missingLocError azurerm.MissingLocationError
		require.ErrorAs(t, err, &missingLocError, "Error should be MissingLocationError type")

		// Check the error message
		assert.Contains(t, err.Error(), "location is required for storage account creation")
	})

	t.Run("NoValidAuthMethodError", func(t *testing.T) {
		t.Parallel()

		// Test the NoValidAuthMethodError type directly since the current bootstrap logic
		// always defaults to Azure AD auth. This tests the error type functionality.
		err := azurerm.NoValidAuthMethodError{}

		// Check the error message
		expectedMsg := "no valid authentication method found: Azure AD auth is recommended. Alternatively, provide one of: MSI, service principal credentials, or SAS token"
		assert.Equal(t, expectedMsg, err.Error())

		// Test that the error can be identified using errors.As
		var noAuthError azurerm.NoValidAuthMethodError
		assert.ErrorAs(t, err, &noAuthError, "Error should be identifiable as NoValidAuthMethodError type")
	})

	t.Run("MissingResourceGroupError", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "mystorageaccount",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			// resource_group_name is missing (required for delete operation)
		}

		b := azurerm.NewBackend()
		err := b.DeleteStorageAccount(t.Context(), l, config, opts)

		require.Error(t, err)

		// Check that the error is of the expected custom type
		var missingRGError azurerm.MissingResourceGroupError
		require.ErrorAs(t, err, &missingRGError, "Error should be MissingResourceGroupError type")

		// Check the error message
		assert.Contains(t, err.Error(), "resource_group_name is required")
	})
}

// TestCustomErrorTypesUnwrapping tests that custom error types properly unwrap underlying errors
func TestCustomErrorTypesUnwrapping(t *testing.T) {
	t.Parallel()

	t.Run("StorageAccountCreationError", func(t *testing.T) {
		t.Parallel()

		// Create a wrapped error to test unwrapping
		underlyingError := errors.New("simulated Azure API error")
		wrappedError := azurerm.StorageAccountCreationError{
			Underlying:         underlyingError,
			StorageAccountName: "testaccount",
		}

		// Test the error message format
		expectedMsg := "error with storage account testaccount: simulated Azure API error"
		assert.Equal(t, expectedMsg, wrappedError.Error())

		// Test that Unwrap() returns the underlying error
		unwrappedError := wrappedError.Unwrap()
		assert.Equal(t, underlyingError, unwrappedError)

		// Test that errors.Is() works with the unwrapped error
		assert.ErrorIs(t, wrappedError, underlyingError)
	})

	t.Run("ContainerCreationError", func(t *testing.T) {
		t.Parallel()

		// Create a wrapped error to test unwrapping
		underlyingError := errors.New("simulated container creation error")
		wrappedError := azurerm.ContainerCreationError{
			Underlying:    underlyingError,
			ContainerName: "testcontainer",
		}

		// Test the error message format
		expectedMsg := "error with container testcontainer: simulated container creation error"
		assert.Equal(t, expectedMsg, wrappedError.Error())

		// Test that Unwrap() returns the underlying error
		unwrappedError := wrappedError.Unwrap()
		assert.Equal(t, underlyingError, unwrappedError)

		// Test that errors.Is() works with the unwrapped error
		assert.ErrorIs(t, wrappedError, underlyingError)
	})

	t.Run("AuthenticationError", func(t *testing.T) {
		t.Parallel()

		// Create a wrapped error to test unwrapping
		underlyingError := errors.New("invalid credentials")
		wrappedError := azurerm.AuthenticationError{
			Underlying: underlyingError,
			AuthMethod: "Azure AD",
		}

		// Test the error message format
		expectedMsg := "Azure authentication failed using Azure AD: invalid credentials"
		assert.Equal(t, expectedMsg, wrappedError.Error())

		// Test that Unwrap() returns the underlying error
		unwrappedError := wrappedError.Unwrap()
		assert.Equal(t, underlyingError, unwrappedError)

		// Test that errors.Is() works with the unwrapped error
		assert.ErrorIs(t, wrappedError, underlyingError)
	})

	t.Run("ContainerDoesNotExist", func(t *testing.T) {
		t.Parallel()

		// Create a wrapped error to test unwrapping
		underlyingError := errors.New("container not found")
		wrappedError := azurerm.ContainerDoesNotExist{
			Underlying:    underlyingError,
			ContainerName: "nonexistent-container",
		}

		// Test the error message format
		expectedMsg := "Container nonexistent-container does not exist. Underlying error: container not found"
		assert.Equal(t, expectedMsg, wrappedError.Error())

		// Test that Unwrap() returns the underlying error
		unwrappedError := wrappedError.Unwrap()
		assert.Equal(t, underlyingError, unwrappedError)

		// Test that errors.Is() works with the unwrapped error
		assert.ErrorIs(t, wrappedError, underlyingError)
	})
}
