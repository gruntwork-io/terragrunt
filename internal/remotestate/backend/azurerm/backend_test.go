// Package azurerm represents Azure storage backend for remote state
package azurerm_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	testing_pkg "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm/testing"
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

// newTestBackend creates a backend instance for testing with mock services
func newTestBackend() *azurerm.Backend {
	return azurerm.NewBackend(testing_pkg.NewTestBackendConfig())
}

func TestNewBackend(t *testing.T) {
	t.Parallel()

	// Create backend with mock config
	b := newTestBackend()
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

	b := newTestBackend()
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
	b := newTestBackend()

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

	// Create a backend instance with mock services
	b := newTestBackend()

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
		// assert.True(t, extConfig.StorageAccountConfig.EnableHierarchicalNS) // removed
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
		b := newTestBackend()
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

			// Create a new backend instance with mock services
			b := newTestBackend()

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

			// Create a new backend instance with mock services
			b := newTestBackend()

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
				// "EnableHierarchicalNS":  false, // removed
				"AccountKind":     "", // Empty means default
				"AccountTier":     "", // Empty means default
				"AccessTier":      "", // Empty means default
				"ReplicationType": "", // Empty means default
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
				"account_kind":                         "BlockBlobStorage",
				"account_tier":                         "Premium",
				"access_tier":                          "Cool",
				"replication_type":                     "ZRS",
				"create_storage_account_if_not_exists": true,
			},
			expectedSAConfig: map[string]interface{}{
				"EnableVersioning":      false,
				"AllowBlobPublicAccess": true,
				// "EnableHierarchicalNS":  true, // removed
				"AccountKind":       "BlockBlobStorage",
				"AccountTier":       "Premium",
				"AccessTier":        "Cool",
				"ReplicationType":   "ZRS",
				"Location":          "eastus",
				"ResourceGroupName": "test-resource-group",
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

			// if v, exists := tc.expectedSAConfig["EnableHierarchicalNS"]; exists && !tc.skipExtendedChecks {
			//     assert.Equal(t, v, azureCfg.StorageAccountConfig.EnableHierarchicalNS)
			// }

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

// Additional comprehensive error path tests for container name validation edge cases
func TestContainerNameValidation_AdditionalEdgeCases(t *testing.T) {
	t.Parallel()

	//nolint: govet
	testCases := []struct {
		name          string
		containerName string
		expectError   bool
		errorMessage  string
	}{
		{
			name:          "three_character_valid",
			containerName: "abc",
			expectError:   false,
		},
		{
			name:          "sixty_three_character_valid",
			containerName: "a" + strings.Repeat("b", 61) + "c", // 63 chars total
			expectError:   false,
		},
		{
			name:          "sixty_four_character_invalid",
			containerName: strings.Repeat("a", 64),
			expectError:   true,
			errorMessage:  "between 3 and 63 characters",
		},
		{
			name:          "two_character_invalid",
			containerName: "ab",
			expectError:   true,
			errorMessage:  "between 3 and 63 characters",
		},
		{
			name:          "one_character_invalid",
			containerName: "a",
			expectError:   true,
			errorMessage:  "between 3 and 63 characters",
		},
		{
			name:          "hyphen_at_start",
			containerName: "-abc",
			expectError:   true,
			errorMessage:  "start and end with a letter or number",
		},
		{
			name:          "hyphen_at_end",
			containerName: "abc-",
			expectError:   true,
			errorMessage:  "start and end with a letter or number",
		},
		{
			name:          "multiple_consecutive_hyphens",
			containerName: "abc--def",
			expectError:   true,
			errorMessage:  "consecutive hyphens",
		},
		{
			name:          "valid_with_hyphens",
			containerName: "a-b-c-d",
			expectError:   false,
		},
		{
			name:          "underscore_invalid",
			containerName: "abc_def",
			expectError:   true,
			errorMessage:  "lowercase letters, numbers, and hyphens",
		},
		{
			name:          "dot_invalid",
			containerName: "abc.def",
			expectError:   true,
			errorMessage:  "lowercase letters, numbers, and hyphens",
		},
		{
			name:          "space_invalid",
			containerName: "abc def",
			expectError:   true,
			errorMessage:  "lowercase letters, numbers, and hyphens",
		},
		{
			name:          "mixed_case_invalid",
			containerName: "abcDef",
			expectError:   true,
			errorMessage:  "lowercase",
		},
		{
			name:          "numbers_only_valid",
			containerName: "123",
			expectError:   false,
		},
		{
			name:          "alphanumeric_valid",
			containerName: "abc123def",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := azurerm.ValidateContainerName(tc.containerName)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test configuration dependency validation for Bootstrap method
func TestBootstrap_ConfigurationDependencyValidation(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name                string
		config              backend.Config
		expectError         bool
		expectedErrorType   interface{}
		expectedErrorString string
	}{
		{
			name: "missing_subscription_id_for_storage_creation",
			config: backend.Config{
				"storage_account_name":                 "testaccount",
				"container_name":                       "test-container",
				"key":                                  "test/terraform.tfstate",
				"location":                             "East US",
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
				// subscription_id missing
			},
			expectError:       true,
			expectedErrorType: &azurerm.MissingSubscriptionIDError{},
		},
		{
			name: "empty_subscription_id_for_storage_creation",
			config: backend.Config{
				"storage_account_name":                 "testaccount",
				"container_name":                       "test-container",
				"key":                                  "test/terraform.tfstate",
				"location":                             "East US",
				"subscription_id":                      "", // Empty
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
			},
			expectError:       true,
			expectedErrorType: &azurerm.MissingSubscriptionIDError{},
		},
		{
			name: "missing_location_for_storage_creation",
			config: backend.Config{
				"storage_account_name":                 "testaccount",
				"container_name":                       "test-container",
				"key":                                  "test/terraform.tfstate",
				"subscription_id":                      "00000000-0000-0000-0000-000000000000",
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
				// location missing
			},
			expectError:       true,
			expectedErrorType: &azurerm.MissingLocationError{},
		},
		{
			name: "empty_location_for_storage_creation",
			config: backend.Config{
				"storage_account_name":                 "testaccount",
				"container_name":                       "test-container",
				"key":                                  "test/terraform.tfstate",
				"subscription_id":                      "00000000-0000-0000-0000-000000000000",
				"location":                             "", // Empty
				"create_storage_account_if_not_exists": true,
				"use_azuread_auth":                     true,
			},
			expectError:       true,
			expectedErrorType: &azurerm.MissingLocationError{},
		},
		{
			name: "valid_config_without_storage_creation",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				// No create_storage_account_if_not_exists, so subscription_id and location not required
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := b.Bootstrap(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrorType != nil {
					require.ErrorAs(t, err, tc.expectedErrorType)
				}
				if tc.expectedErrorString != "" {
					require.Contains(t, err.Error(), tc.expectedErrorString)
				}
			}
		})
	}
}

// Test authentication configuration error paths in Bootstrap
func TestBootstrap_AuthenticationConfigurationErrors(t *testing.T) {
	// Note: Cannot use t.Parallel() here because we use t.Setenv()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name                string
		config              backend.Config
		envVars             map[string]string
		expectError         bool
		expectedErrorType   interface{}
		expectedErrorString string
	}{
		{
			name: "no_authentication_method",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				// No authentication method specified and use_azuread_auth defaults to true
			},
			expectError: false, // Azure AD auth should be used by default
		},
		{
			name: "explicit_no_auth_methods",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     false,
				"use_msi":              false,
				// No other auth methods - should still default to Azure AD auth
			},
			envVars: map[string]string{
				// Clear any existing auth environment variables
				"AZURE_CLIENT_ID":         "",
				"AZURE_CLIENT_SECRET":     "",
				"AZURE_TENANT_ID":         "",
				"AZURE_SUBSCRIPTION_ID":   "",
				"AZURE_STORAGE_SAS_TOKEN": "",
				"ARM_ACCESS_KEY":          "",
				"AZURE_STORAGE_KEY":       "",
			},
			expectError: false, // Azure AD auth is now the default and will be used
		},
		{
			name: "incomplete_service_principal_config",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"client_id":            "client-id",
				"tenant_id":            "tenant-id",
				// Missing client_secret and subscription_id
				"use_azuread_auth": false,
			},
			expectError: false, // Will fall back to Azure AD auth by default
		},
		{
			name: "invalid_sas_token_format",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"sas_token":            "invalid-sas-token", // Invalid format
				"use_azuread_auth":     false,
			},
			expectError: false, // SAS token validation happens in Azure SDK, not in our validation
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() here because we use t.Setenv()

			// Set environment variables for the test
			if tc.envVars != nil {
				for key, value := range tc.envVars {
					if value == "" {
						t.Setenv(key, "")
					} else {
						t.Setenv(key, value)
					}
				}
			}

			err := b.Bootstrap(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrorType != nil {
					require.ErrorAs(t, err, tc.expectedErrorType)
				}
				if tc.expectedErrorString != "" {
					require.Contains(t, err.Error(), tc.expectedErrorString)
				}
			}
		})
	}
}

// Test error paths for Delete method
func TestDelete_ErrorPathsDetailed(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name                string
		config              backend.Config
		expectError         bool
		expectedErrorType   interface{}
		expectedErrorString string
	}{
		{
			name: "missing_storage_account_name",
			config: backend.Config{
				"container_name":   "test-container",
				"key":              "test/terraform.tfstate",
				"use_azuread_auth": true,
				// storage_account_name missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "storage_account_name",
		},
		{
			name: "empty_storage_account_name",
			config: backend.Config{
				"storage_account_name": "", // Empty
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "storage_account_name",
		},
		{
			name: "missing_container_name",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				// container_name missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "container_name",
		},
		{
			name: "missing_key",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"use_azuread_auth":     true,
				// key missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "key",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := b.Delete(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrorType != nil {
					var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
					require.ErrorAs(t, err, &missingConfigError)
				}
				if tc.expectedErrorString != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorString)
				}
			}
		})
	}
}

// Test error paths for DeleteContainer method
func TestDeleteContainer_ErrorPathsDetailed(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name                string
		config              backend.Config
		expectError         bool
		expectedErrorType   interface{}
		expectedErrorString string
	}{
		{
			name: "missing_storage_account_name",
			config: backend.Config{
				"container_name":   "test-container",
				"key":              "test/terraform.tfstate",
				"use_azuread_auth": true,
				// storage_account_name missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "storage_account_name",
		},
		{
			name: "missing_container_name",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				// container_name missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "container_name",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := b.DeleteContainer(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrorType != nil {
					var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
					require.ErrorAs(t, err, &missingConfigError)
				}
				if tc.expectedErrorString != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorString)
				}
			}
		})
	}
}

// Test error paths for Migrate method
func TestMigrate_ErrorPathsDetailed(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name                string
		srcConfig           backend.Config
		dstConfig           backend.Config
		expectError         bool
		expectedErrorType   interface{}
		expectedErrorString string
	}{
		{
			name: "invalid_source_config_missing_storage_account",
			srcConfig: backend.Config{
				"container_name":   "src-container",
				"key":              "test/terraform.tfstate",
				"use_azuread_auth": true,
				// storage_account_name missing
			},
			dstConfig: backend.Config{
				"storage_account_name": "dstaccount",
				"container_name":       "dst-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "storage_account_name",
		},
		{
			name: "invalid_destination_config_missing_container",
			srcConfig: backend.Config{
				"storage_account_name": "srcaccount",
				"container_name":       "src-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			dstConfig: backend.Config{
				"storage_account_name": "dstaccount",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
				// container_name missing
			},
			expectError:         true,
			expectedErrorType:   azurerm.MissingRequiredAzureRemoteStateConfig(""),
			expectedErrorString: "container_name",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := b.Migrate(t.Context(), l, tc.srcConfig, tc.dstConfig, opts)
			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrorType != nil {
					var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
					require.ErrorAs(t, err, &missingConfigError)
				}
				if tc.expectedErrorString != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorString)
				}
			}
		})
	}
}

// TestNeedsBootstrap_ConfigValidation tests configuration validation in NeedsBootstrap
// This test focuses only on config parsing errors that happen before Azure API calls
func TestNeedsBootstrap_ConfigValidation(t *testing.T) {
	t.Parallel()
	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	b := azurerm.NewBackend(nil)

	//nolint: govet
	testCases := []struct {
		name          string
		config        backend.Config
		expectError   bool
		expectedError string
	}{
		{
			name: "missing_storage_account_name",
			config: backend.Config{
				"container_name":   "test-container",
				"key":              "test/terraform.tfstate",
				"use_azuread_auth": true,
			},
			expectError:   true,
			expectedError: "storage_account_name",
		},
		{
			name: "empty_storage_account_name",
			config: backend.Config{
				"storage_account_name": "",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError:   true,
			expectedError: "storage_account_name",
		},
		{
			name: "missing_container_name",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError:   true,
			expectedError: "container_name",
		},
		{
			name: "empty_container_name",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "",
				"key":                  "test/terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError:   true,
			expectedError: "container_name",
		},
		{
			name: "missing_key",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"use_azuread_auth":     true,
			},
			expectError:   true,
			expectedError: "key",
		},
		{
			name: "empty_key",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "",
				"use_azuread_auth":     true,
			},
			expectError:   true,
			expectedError: "key",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// These tests only exercise config parsing, which should fail before Azure API calls
			_, err := b.NeedsBootstrap(t.Context(), l, tc.config, opts)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeleteStorageAccount_ConfigValidation tests additional configuration validation for delete operations
func TestDeleteStorageAccount_ConfigValidation(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	b := azurerm.NewBackend(nil)

	t.Run("missing_storage_account_name", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"subscription_id":     "00000000-0000-0000-0000-000000000000",
			"resource_group_name": "test-rg",
			"container_name":      "test-container",
			"key":                 "test/terraform.tfstate",
			"use_azuread_auth":    true,
			// storage_account_name missing
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)

		var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
		require.ErrorAs(t, err, &missingConfigError)
		assert.Equal(t, "storage_account_name", string(missingConfigError))
	})

	t.Run("empty_storage_account_name", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "", // Empty
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":  "test-rg",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)

		var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
		require.ErrorAs(t, err, &missingConfigError)
		assert.Equal(t, "storage_account_name", string(missingConfigError))
	})

	t.Run("missing_subscription_id_for_delete", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "testaccount",
			"resource_group_name":  "test-rg",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			// subscription_id missing
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)

		var missingSubError azurerm.MissingSubscriptionIDError
		assert.ErrorAs(t, err, &missingSubError)
	})

	t.Run("missing_resource_group_for_delete", func(t *testing.T) {
		t.Parallel()

		config := backend.Config{
			"storage_account_name": "testaccount",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			// resource_group_name missing
		}

		err := b.DeleteStorageAccount(t.Context(), l, config, opts)
		require.Error(t, err)

		var missingResourceGroupError azurerm.MissingResourceGroupError
		assert.ErrorAs(t, err, &missingResourceGroupError)
	})
}
