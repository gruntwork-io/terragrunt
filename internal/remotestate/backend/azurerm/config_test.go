package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				"container_tags": map[string]string{
					"Environment": "Test",
				},
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
				"container_tags": map[string]string{
					"Environment": "Test",
				},
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
			name: "with-container-tags",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
					"Project":     "Terragrunt",
				},
			},
			expectError: false,
		},
		{
			name: "with-invalid-container-tags",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"container_tags":       "invalid-tags", // Should be a map[string]string
			},
			expectError: true,
		},
		{
			name: "with-all-extended-features",
			config: azurerm.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
				"key":                  "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
					"Project":     "Terragrunt",
				},
				"connection_string": "test-connection-string",
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

				// Verify extended settings if present
				if tags, ok := tc.config["container_tags"].(map[string]string); ok {
					assert.Equal(t, tags, config.ContainerTags)
				}
			}
		})
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
			name: "with-container-tags",
			config: &azurerm.ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: azurerm.RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:      "test-container",
					Key:                "test/terraform.tfstate",
				},
				ContainerTags: map[string]string{
					"Environment": "Test",
					"Project":     "Terragrunt",
				},
			},
			expectError: false,
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
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
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
