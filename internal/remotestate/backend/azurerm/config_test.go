package azurerm_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupOptionsWithExperiment creates TerragruntOptions with specified experiment enabled
func setupOptionsWithExperiment(experimentName string) *options.TerragruntOptions {
	opts := options.NewTerragruntOptions()
	opts.Experiments.EnableExperiment(experimentName)
	return opts
}

// setupWithAzureBackendExperiment enables Azure backend experiment and registers backends
// This is needed for tests that expect the Azure backend to be available
func setupWithAzureBackendExperiment() *options.TerragruntOptions {
	opts := setupOptionsWithExperiment(experiment.AzureBackend)
	remotestate.RegisterBackends(opts)
	return opts
}

// Helper function to assert that the expected arguments are contained in actualArgs
func assertTerraformInitArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	t.Helper()

	expected := strings.Split(expectedArgs, " ")
	assert.ElementsMatch(t, actualArgs, expected, "elements differ")
}

func TestFilterOutTerragruntKeys(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		config   azurerm.Config
		expected azurerm.Config
		name     string
	}{
		{
			name: "no-terragrunt-keys",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
			},
			expected: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
			},
		},
		{
			name: "with-terragrunt-keys",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				// Terragrunt-specific keys to be filtered out
				"create_storage_account_if_not_exists": true,
				"enable_versioning":                    true,
				"location":                             "eastus",
				"allow_blob_public_access":             false,
				"skip_storage_account_update":          false,
				"account_kind":                         "StorageV2",
				"account_tier":                         "Standard",
				"access_tier":                          "Hot",
				"replication_type":                     "LRS",
			},
			expected: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filtered := tc.config.FilterOutTerragruntKeys()
			// Convert filtered map[string]interface{} to Config for comparison
			filteredConfig := azurerm.Config(filtered)
			assert.Equal(t, tc.expected, filteredConfig)
		})
	}
}

func TestParseExtendedAzureConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config      azurerm.Config
		name        string
		expectError bool
	}{
		{
			name: "valid-config",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
			},
			expectError: false,
		},
		{
			name: "with-connection-string",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"connection_string":    "test-connection-string",
			},
			expectError: false,
		},
		{
			name: "with-all-extended-features",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"connection_string":    "test-connection-string",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config, err := tc.config.ParseExtendedAzureConfig()
			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, config)

				// Verify core settings
				assert.Equal(t, tc.config["storage_account_name"], config.RemoteStateConfigAzurerm.StorageAccountName)
				assert.Equal(t, tc.config["container_name"], config.RemoteStateConfigAzurerm.ContainerName)
				assert.Equal(t, tc.config["key"], config.RemoteStateConfigAzurerm.Key)

			}
		},
		)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config      *azurerm.ExtendedRemoteStateConfigAzurerm // pointer type (8 bytes)
		name        string                                    // string (16 bytes)
		expectError bool                                      // bool (1 byte)
	}{
		{
			name: "valid-config",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
				},
			},
			expectError: false,
		},
		{
			name: "missing-storage-account",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					ContainerName: "test-container",
					Key:           "test/terraform.tfstate",
				},
			},
			expectError: true,
		},
		{
			name: "missing-container",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					Key:                "test/terraform.tfstate",
				},
			},
			expectError: true,
		},
		{
			name: "missing-key",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
				},
			},
			expectError: true,
		},
		{
			name: "with-environment-specified",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
					Environment:        "usgovernment",
				},
			},
			expectError: false,
		},
		{
			name: "with-resource-group-name",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
					ResourceGroupName:  "terragrunt-test",
				},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.config.Validate()
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCacheKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string                                   // string (16 bytes) - first for alignment
		expected string                                   // string (16 bytes) - group strings together
		config   azurerm.ExtendedRemoteStateConfigAzurerm // largest field at end
	}{
		{
			config: azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
				},
			},
			expected: "testaccount-test-container-test/terraform.tfstate",
			name:     "basic-config",
		},
		{
			config: azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
					ResourceGroupName:  "test-rg",
				},
			},
			expected: "testaccount-test-container-test/terraform.tfstate",
			name:     "with-resource-group",
		},
		{
			config: azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "env/prod/region/us-east-1/terraform.tfstate",
				},
			},
			expected: "testaccount-test-container-env/prod/region/us-east-1/terraform.tfstate",
			name:     "with-nested-key",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.config.CacheKey(), "Cache key mismatch for %s", tc.name)
		})
	}
}
func TestAzurermTFInitArgs(t *testing.T) {
	t.Parallel()

	// Set up options with Azure backend experiment enabled
	setupWithAzureBackendExperiment()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			// Standard Azure backend parameters that should be passed to terraform
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"use_azuread_auth":     true,
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"tenant_id":            "00000000-0000-0000-0000-000000000000",
			"resource_group_name":  "my-resource-group", // Will be filtered out
			"environment":          "public",

			// Terragrunt-specific options that should be filtered out
			"create_storage_account_if_not_exists": true,
			"enable_versioning":                    true,
			"location":                             "eastus",
			"allow_blob_public_access":             false,
			"account_kind":                         "StorageV2",
			"account_tier":                         "Standard",
			"replication_type":                     "LRS",
			"storage_account_tags": map[string]any{
				"Environment": "Dev",
				"Owner":       "Terragrunt",
			},
		},
	}

	args := remotestate.New(cfg).GetTFInitArgs()

	// Verify that only the standard Azure backend parameters are passed to terraform
	// and all Terragrunt-specific options are filtered out
	assertTerraformInitArgsEqual(t, args, "-backend-config=storage_account_name=mystorageaccount "+
		"-backend-config=container_name=terraform-state "+
		"-backend-config=key=terraform.tfstate "+
		"-backend-config=use_azuread_auth=true "+
		"-backend-config=subscription_id=00000000-0000-0000-0000-000000000000 "+
		"-backend-config=tenant_id=00000000-0000-0000-0000-000000000000 "+
		"-backend-config=environment=public")

	// Verify that resource_group_name is filtered out since it's in the terragruntOnlyConfigs list
	for _, arg := range args {
		assert.NotContains(t, arg, "resource_group_name", "resource_group_name should be filtered out")
		assert.NotContains(t, arg, "create_storage_account_if_not_exists", "create_storage_account_if_not_exists should be filtered out")
		assert.NotContains(t, arg, "enable_versioning", "enable_versioning should be filtered out")
		assert.NotContains(t, arg, "location", "location should be filtered out")
	}
}

// TestFilterOutTerragruntKeysAzure tests the FilterOutTerragruntKeys function directly
func TestFilterOutTerragruntKeysAzure(t *testing.T) {
	t.Parallel()

	// Create a config with a mix of standard and Terragrunt-specific options
	config := azurerm.Config{
		"storage_account_name":                 "mystorageaccount",
		"container_name":                       "terraform-state",
		"key":                                  "terraform.tfstate",
		"use_azuread_auth":                     true,
		"subscription_id":                      "00000000-0000-0000-0000-000000000000",
		"tenant_id":                            "00000000-0000-0000-0000-000000000000",
		"create_storage_account_if_not_exists": true,
		"enable_versioning":                    true,
		"location":                             "eastus",
		"resource_group_name":                  "my-resource-group",
	}

	// Filter out Terragrunt-specific keys
	filtered := config.FilterOutTerragruntKeys()

	// Verify that only standard Azure backend parameters remain
	assert.Contains(t, filtered, "storage_account_name")
	assert.Contains(t, filtered, "container_name")
	assert.Contains(t, filtered, "key")
	assert.Contains(t, filtered, "use_azuread_auth")
	assert.Contains(t, filtered, "subscription_id")
	assert.Contains(t, filtered, "tenant_id")

	// Verify that Terragrunt-specific options are filtered out
	assert.NotContains(t, filtered, "create_storage_account_if_not_exists")
	assert.NotContains(t, filtered, "enable_versioning")
	assert.NotContains(t, filtered, "location")
	assert.NotContains(t, filtered, "resource_group_name")
}

// TestParseExtendedAzureConfigValidation tests the parsing of extended Azure configuration
func TestParseExtendedAzureConfigValidation(t *testing.T) {
	t.Parallel()

	// Create a config with both basic and extended options
	config := azurerm.Config{
		"storage_account_name":                 "mystorageaccount",
		"container_name":                       "terraform-state",
		"key":                                  "terraform.tfstate",
		"use_azuread_auth":                     true,
		"create_storage_account_if_not_exists": true,
		"enable_versioning":                    true,
		"location":                             "eastus",
	}

	// Parse the config
	parsed, err := config.ParseExtendedAzureConfig()
	require.NoError(t, err)

	// Verify basic options
	assert.Equal(t, "mystorageaccount", parsed.RemoteStateConfigAzurerm.StorageAccountName)
	assert.Equal(t, "terraform-state", parsed.RemoteStateConfigAzurerm.ContainerName)
	assert.Equal(t, "terraform.tfstate", parsed.RemoteStateConfigAzurerm.Key)
	assert.True(t, parsed.RemoteStateConfigAzurerm.UseAzureADAuth)

	// Verify extended options
	assert.True(t, parsed.StorageAccountConfig.CreateStorageAccountIfNotExists)
	assert.True(t, parsed.StorageAccountConfig.EnableVersioning)
	assert.Equal(t, "eastus", parsed.StorageAccountConfig.Location)
}

// TestBackendConfigValidation tests the validation of backend configuration
func TestBackendConfigValidation(t *testing.T) {
	t.Parallel()

	// Generate a unique suffix for storage account names to avoid conflicts
	timestampStr := strconv.FormatInt(time.Now().UnixNano(), 10)
	uniqueSuffix := timestampStr[len(timestampStr)-10:] // Last 10 digits of timestamp

	// Create a unique storage account name for tests
	// Azure storage account names must be between 3-24 characters, lowercase letters and numbers only
	uniqueStorageAcct := "tgtest" + uniqueSuffix[:8] // Keep within 24 char limit

	testCases := []struct {
		// Map first (larger alignment requirements)
		config backend.Config
		// String fields
		name     string
		errorMsg string
		// Boolean field last
		expectError bool
	}{
		{
			name: "valid-config",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct,
				"container_name":       "terraform-state",
				"key":                  "terraform.tfstate",
				"use_azuread_auth":     true,
			},
			expectError: false,
		},
		{
			name: "missing-storage-account",
			config: backend.Config{
				"container_name": "terraform-state",
				"key":            "terraform.tfstate",
			},
			expectError: true,
			errorMsg:    "storage_account_name",
		},
		{
			name: "missing-container",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct + "1",
				"key":                  "terraform.tfstate",
			},
			expectError: true,
			errorMsg:    "container_name",
		},
		{
			name: "missing-key",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct + "2",
				"container_name":       "terraform-state",
			},
			expectError: true,
			errorMsg:    "key",
		},
		{
			name: "with-environment-specified",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct + "3",
				"container_name":       "terraform-state",
				"key":                  "terraform.tfstate",
				"environment":          "usgovernment",
				"use_azuread_auth":     true,
			},
			expectError: false,
		},
		{
			name: "with-resource-group-name",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct + "4",
				"container_name":       "terraform-state",
				"key":                  "terraform.tfstate",
				"resource_group_name":  "my-resource-group",
				"use_azuread_auth":     true,
			},
			expectError: false,
		},
		{
			name: "with-multiple-auth-methods",
			config: backend.Config{
				"storage_account_name": uniqueStorageAcct + "5",
				"container_name":       "terraform-state",
				"key":                  "terraform.tfstate",
				"use_azuread_auth":     true,
				"use_msi":              true,
			},
			expectError: true,
			errorMsg:    "multiple authentication methods",
		},
		{
			name: "with-all-storage-account-bootstrap-options",
			config: backend.Config{
				"storage_account_name":                 uniqueStorageAcct + "6",
				"container_name":                       "terraform-state",
				"key":                                  "terraform.tfstate",
				"subscription_id":                      "00000000-0000-0000-0000-000000000000",
				"resource_group_name":                  "my-resource-group",
				"location":                             "eastus",
				"create_storage_account_if_not_exists": true,
				"enable_versioning":                    true,
				"allow_blob_public_access":             false,
				"account_kind":                         "StorageV2",
				"account_tier":                         "Standard",
				"replication_type":                     "LRS",
				"use_azuread_auth":                     true,
			},
			expectError: false,
		},
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're in non-interactive mode to prevent any prompts
	opts.NonInteractive = true

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// IMPORTANT: Instead of actually checking if the backend needs bootstrapping
			// (which would make API calls), just validate the configuration
			azureCfg, err := azurerm.Config(tc.config).ExtendedAzureConfig()

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
				// Additional validation that the config was parsed properly
				if err == nil {
					assert.NotEmpty(t, azureCfg.RemoteStateConfigAzurerm.StorageAccountName)
					assert.NotEmpty(t, azureCfg.RemoteStateConfigAzurerm.ContainerName)
					assert.NotEmpty(t, azureCfg.RemoteStateConfigAzurerm.Key)
				}
			}
		})
	}
}
