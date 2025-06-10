package azurerm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterOutTerragruntKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		config   Config
		expected Config
	}{
		{
			name: "no-terragrunt-keys",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
			expected: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
		},
		{
			name: "with-terragrunt-keys",
			config: Config{
				"storage_account_name":      "testaccount",
				"container_name":           "test-container",
				"key":                     "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
				},
			},
			expected: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filtered := tc.config.FilterOutTerragruntKeys()
			// Convert filtered map[string]interface{} to Config for comparison
			filteredConfig := Config(filtered)
			assert.Equal(t, tc.expected, filteredConfig)
		})
	}
}

func TestParseExtendedAzureConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid-config",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
				},
			},
			expectError: false,
		},
		{
			name: "with-connection-string",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"connection_string":  "test-connection-string",
			},
			expectError: false,
		},

		{
			name: "with-container-tags",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
					"Project":    "Terragrunt",
				},
			},
			expectError: false,
		},
		{
			name: "with-invalid-container-tags",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags":     "invalid-tags", // Should be a map[string]string
			},
			expectError: true,
		},
		{
			name: "with-all-extended-features",
			config: Config{
				"storage_account_name": "testaccount",
				"container_name":      "test-container",
				"key":                "test/terraform.tfstate",
				"container_tags": map[string]string{
					"Environment": "Test",
					"Project":    "Terragrunt",
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
				assert.Error(t, err)
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
		name        string
		config      *ExtendedRemoteStateConfigAzurerm
		expectError bool
	}{
		{
			name: "valid-config",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
				},
			},
			expectError: false,
		},
		{
			name: "missing-storage-account",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					ContainerName: "test-container",
					Key:          "test/terraform.tfstate",
				},
			},
			expectError: true,
		},
		{
			name: "missing-container",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					Key:               "test/terraform.tfstate",
				},
			},
			expectError: true,
		},
		{
			name: "missing-key",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
				},
			},
			expectError: true,
		},

		{
			name: "with-container-tags",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
				},
				ContainerTags: map[string]string{
					"Environment": "Test",
					"Project":    "Terragrunt",
				},
			},
			expectError: false,
		},
		{
			name: "with-environment-specified",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
					Environment:      "usgovernment",
				},
			},
			expectError: false,
		},
		{
			name: "with-resource-group-name",
			config: &ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
					ResourceGroupName: "terragrunt-test",
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
		name       string
		config     ExtendedRemoteStateConfigAzurerm
		expected   string
	}{
		{
			name: "basic-config",
			config: ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
				},
			},
			expected: "testaccount-test-container-test/terraform.tfstate",
		},
		{
			name: "with-resource-group",
			config: ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "test/terraform.tfstate",
					ResourceGroupName: "test-rg",
				},
			},
			expected: "testaccount-test-container-test/terraform.tfstate",
		},
		{
			name: "with-nested-key",
			config: ExtendedRemoteStateConfigAzurerm{
				RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
					StorageAccountName: "testaccount",
					ContainerName:     "test-container",
					Key:              "env/prod/region/us-east-1/terraform.tfstate",
				},
			},
			expected: "testaccount-test-container-env/prod/region/us-east-1/terraform.tfstate",
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
