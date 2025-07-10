package azurehelper_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStringPtr tests the StringPtr utility function
func TestStringPtr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "regular string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "string with spaces",
			input:    "test string with spaces",
			expected: "test string with spaces",
		},
		{
			name:     "string with special characters",
			input:    "test@#$%^&*()",
			expected: "test@#$%^&*()",
		},
		{
			name:     "unicode string",
			input:    "测试字符串", //nolint:gosmopolitan
			expected: "测试字符串", //nolint:gosmopolitan
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.StringPtr(tc.input)
			require.NotNil(t, result)
			assert.Equal(t, tc.expected, *result)
		})
	}
}

// TestConvertAzureError tests the ConvertAzureError utility function
func TestConvertAzureError(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name           string
		input          error
		expectedResult *azurehelper.AzureResponseError
	}{
		{
			name:           "nil error",
			input:          nil,
			expectedResult: nil,
		},
		{
			name:           "non-Azure error",
			input:          errors.Errorf("regular error"),
			expectedResult: nil,
		},
		{
			name: "Azure ResponseError",
			input: &azcore.ResponseError{
				StatusCode: 404,
				ErrorCode:  "NotFound",
			},
			expectedResult: &azurehelper.AzureResponseError{
				StatusCode: 404,
				ErrorCode:  "NotFound",
				Message:    "404 NotFound", // This will be what Error() returns
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.ConvertAzureError(tc.input)
			if tc.expectedResult == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tc.expectedResult.StatusCode, result.StatusCode)
				assert.Equal(t, tc.expectedResult.ErrorCode, result.ErrorCode)
				// Message contains the error string, just check it's not empty
				assert.NotEmpty(t, result.Message)
			}
		})
	}
}

// TestAzureResponseErrorString tests the Error() method of AzureResponseError
func TestAzureResponseErrorString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      *azurehelper.AzureResponseError
		expected string
	}{
		{
			name: "complete error",
			err: &azurehelper.AzureResponseError{
				StatusCode: 404,
				ErrorCode:  "NotFound",
				Message:    "Resource not found",
			},
			expected: "Azure API error (StatusCode=404, ErrorCode=NotFound): Resource not found",
		},
		{
			name: "error with empty message",
			err: &azurehelper.AzureResponseError{
				StatusCode: 500,
				ErrorCode:  "InternalError",
				Message:    "",
			},
			expected: "Azure API error (StatusCode=500, ErrorCode=InternalError): ",
		},
		{
			name: "error with empty error code",
			err: &azurehelper.AzureResponseError{
				StatusCode: 400,
				ErrorCode:  "",
				Message:    "Bad request",
			},
			expected: "Azure API error (StatusCode=400, ErrorCode=): Bad request",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.err.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetStorageAccountSKU tests the GetStorageAccountSKU utility function
func TestGetStorageAccountSKU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		accountTier       string
		replicationType   string
		expectedSKU       string
		expectedIsDefault bool
	}{
		{
			name:              "both empty - use defaults",
			accountTier:       "",
			replicationType:   "",
			expectedSKU:       "Standard_LRS",
			expectedIsDefault: true,
		},
		{
			name:              "tier empty - use default tier",
			accountTier:       "",
			replicationType:   "GRS",
			expectedSKU:       "Standard_GRS",
			expectedIsDefault: false,
		},
		{
			name:              "replication empty - use default replication",
			accountTier:       "Premium",
			replicationType:   "",
			expectedSKU:       "Premium_LRS",
			expectedIsDefault: false,
		},
		{
			name:              "both specified",
			accountTier:       "Standard",
			replicationType:   "ZRS",
			expectedSKU:       "Standard_ZRS",
			expectedIsDefault: false,
		},
		{
			name:              "premium with ZRS",
			accountTier:       "Premium",
			replicationType:   "ZRS",
			expectedSKU:       "Premium_ZRS",
			expectedIsDefault: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resultSKU, resultIsDefault := azurehelper.GetStorageAccountSKU(tc.accountTier, tc.replicationType)
			assert.Equal(t, tc.expectedSKU, resultSKU)
			assert.Equal(t, tc.expectedIsDefault, resultIsDefault)
		})
	}
}

// TestCompareStringMaps tests the CompareStringMaps utility function
func TestCompareStringMaps(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name     string
		existing map[string]*string
		desired  map[string]string
		expected bool
	}{
		{
			name:     "both empty",
			existing: map[string]*string{},
			desired:  map[string]string{},
			expected: true,
		},
		{
			name:     "existing empty, desired not empty",
			existing: map[string]*string{},
			desired:  map[string]string{"key1": "value1"},
			expected: false,
		},
		{
			name:     "existing not empty, desired empty",
			existing: map[string]*string{"key1": azurehelper.StringPtr("value1")},
			desired:  map[string]string{},
			expected: false,
		},
		{
			name: "identical maps",
			existing: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
				"key2": azurehelper.StringPtr("value2"),
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: true,
		},
		{
			name: "different values",
			existing: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
				"key2": azurehelper.StringPtr("value2"),
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "different_value",
			},
			expected: false,
		},
		{
			name: "missing key in existing",
			existing: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name: "nil value in existing",
			existing: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
				"key2": nil,
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name: "extra key in existing",
			existing: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
				"key2": azurehelper.StringPtr("value2"),
				"key3": azurehelper.StringPtr("value3"),
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.CompareStringMaps(tc.existing, tc.desired)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestConvertToPointerMap tests the ConvertToPointerMap utility function
func TestConvertToPointerMap(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]*string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]*string{},
		},
		{
			name: "single key-value pair",
			input: map[string]string{
				"key1": "value1",
			},
			expected: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
			},
		},
		{
			name: "multiple key-value pairs",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			expected: map[string]*string{
				"key1": azurehelper.StringPtr("value1"),
				"key2": azurehelper.StringPtr("value2"),
				"key3": azurehelper.StringPtr("value3"),
			},
		},
		{
			name: "empty values",
			input: map[string]string{
				"key1": "",
				"key2": "value2",
			},
			expected: map[string]*string{
				"key1": azurehelper.StringPtr(""),
				"key2": azurehelper.StringPtr("value2"),
			},
		},
		{
			name: "special characters",
			input: map[string]string{
				"key@1":     "value#1",
				"key space": "value with spaces",
			},
			expected: map[string]*string{
				"key@1":     azurehelper.StringPtr("value#1"),
				"key space": azurehelper.StringPtr("value with spaces"),
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.ConvertToPointerMap(tc.input)
			require.Len(t, result, len(tc.expected))

			for key, expectedValue := range tc.expected {
				resultValue, exists := result[key]
				assert.True(t, exists, "Key %s should exist in result", key)
				if expectedValue == nil {
					assert.Nil(t, resultValue)
				} else {
					require.NotNil(t, resultValue)
					assert.Equal(t, *expectedValue, *resultValue)
				}
			}
		})
	}
}

// TestCompareAccessTier tests the CompareAccessTier utility function
func TestCompareAccessTier(t *testing.T) {
	t.Parallel()

	hotTier := armstorage.AccessTierHot
	coolTier := armstorage.AccessTierCool
	premiumTier := armstorage.AccessTierPremium

	tests := []struct {
		name     string
		current  *armstorage.AccessTier
		desired  string
		expected bool
	}{
		{
			name:     "both nil/empty",
			current:  nil,
			desired:  "",
			expected: true,
		},
		{
			name:     "current nil, desired not empty",
			current:  nil,
			desired:  "Hot",
			expected: false,
		},
		{
			name:     "current not nil, desired empty",
			current:  &hotTier,
			desired:  "",
			expected: false,
		},
		{
			name:     "matching hot tier",
			current:  &hotTier,
			desired:  "Hot",
			expected: true,
		},
		{
			name:     "matching cool tier",
			current:  &coolTier,
			desired:  "Cool",
			expected: true,
		},
		{
			name:     "matching premium tier",
			current:  &premiumTier,
			desired:  "Premium",
			expected: true,
		},
		{
			name:     "different tiers",
			current:  &hotTier,
			desired:  "Cool",
			expected: false,
		},
		{
			name:     "case sensitivity",
			current:  &hotTier,
			desired:  "hot",
			expected: false,
		},
		{
			name:     "invalid desired tier",
			current:  &hotTier,
			desired:  "InvalidTier",
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.CompareAccessTier(tc.current, tc.desired)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDefaultStorageAccountConfig tests the DefaultStorageAccountConfig function
func TestDefaultStorageAccountConfig(t *testing.T) {
	t.Parallel()

	config := azurehelper.DefaultStorageAccountConfig()

	// Verify that the default config has expected values
	assert.Equal(t, "Standard", config.AccountTier)
	assert.Equal(t, "LRS", config.ReplicationType)
	assert.Equal(t, "StorageV2", config.AccountKind)
	assert.Equal(t, azurehelper.AccessTierHot, config.AccessTier)
	assert.True(t, config.EnableVersioning)
	assert.False(t, config.AllowBlobPublicAccess)

	// Verify that required fields are empty (to be filled by caller)
	assert.Empty(t, config.SubscriptionID)
	assert.Empty(t, config.ResourceGroupName)
	assert.Empty(t, config.StorageAccountName)
	assert.Empty(t, config.Location)

	// Verify that optional fields have reasonable defaults
	assert.NotNil(t, config.Tags)
	assert.Equal(t, "terragrunt", config.Tags["created-by"])
}

// TestGenerateUUID tests the GenerateUUID utility function
func TestGenerateUUID(t *testing.T) {
	t.Parallel()

	// Test basic properties of generated UUIDs
	t.Run("generates non-empty UUID", func(t *testing.T) {
		t.Parallel()

		uuid := azurehelper.GenerateUUID()
		assert.NotEmpty(t, uuid)
	})

	t.Run("generates UUID with correct format", func(t *testing.T) {
		t.Parallel()

		uuid := azurehelper.GenerateUUID()
		// UUID format: 8-4-4-4-12 characters
		assert.Len(t, uuid, 36) // 8+4+4+4+12+4 hyphens = 36
		assert.Equal(t, "-", string(uuid[8]))
		assert.Equal(t, "-", string(uuid[13]))
		assert.Equal(t, "-", string(uuid[18]))
		assert.Equal(t, "-", string(uuid[23]))
	})

	t.Run("generates different UUIDs on subsequent calls", func(t *testing.T) {
		t.Parallel()

		uuid1 := azurehelper.GenerateUUID()
		uuid2 := azurehelper.GenerateUUID()
		assert.NotEqual(t, uuid1, uuid2)
	})

	t.Run("generates UUID with only hex characters and hyphens", func(t *testing.T) {
		t.Parallel()

		uuid := azurehelper.GenerateUUID()
		for i, char := range uuid {
			if i == 8 || i == 13 || i == 18 || i == 23 {
				assert.Equal(t, '-', char, "Character at position %d should be hyphen", i)
			} else {
				assert.True(t,
					(char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
					"Character '%c' at position %d should be hex digit", char, i)
			}
		}
	})

	t.Run("generates multiple unique UUIDs", func(t *testing.T) {
		t.Parallel()

		numUUIDs := 50
		uuids := make(map[string]bool)

		for i := 0; i < numUUIDs; i++ {
			uuid := azurehelper.GenerateUUID()
			assert.False(t, uuids[uuid], "UUID %s was generated more than once", uuid)
			uuids[uuid] = true
		}

		assert.Len(t, uuids, numUUIDs)
	})
}

// TestStorageAccountConfigValidate tests the Validate method of StorageAccountConfig
func TestStorageAccountConfigValidate(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name        string
		config      azurehelper.StorageAccountConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid complete config",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "12345678-1234-1234-1234-123456789012",
				ResourceGroupName:  "test-rg",
				StorageAccountName: "teststorageaccount",
				Location:           "East US",
			},
			expectError: false,
		},
		{
			name: "missing subscription ID",
			config: azurehelper.StorageAccountConfig{
				ResourceGroupName:  "test-rg",
				StorageAccountName: "teststorageaccount",
				Location:           "East US",
			},
			expectError: true,
			errorMsg:    "subscription_id is required",
		},
		{
			name: "missing resource group name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "12345678-1234-1234-1234-123456789012",
				StorageAccountName: "teststorageaccount",
				Location:           "East US",
			},
			expectError: true,
			errorMsg:    "resource_group_name is required",
		},
		{
			name: "missing storage account name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:    "12345678-1234-1234-1234-123456789012",
				ResourceGroupName: "test-rg",
				Location:          "East US",
			},
			expectError: true,
			errorMsg:    "storage_account_name is required",
		},
		{
			name: "missing location",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "12345678-1234-1234-1234-123456789012",
				ResourceGroupName:  "test-rg",
				StorageAccountName: "teststorageaccount",
			},
			expectError: true,
			errorMsg:    "location is required",
		},
		{
			name: "empty strings should fail validation",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "",
				ResourceGroupName:  "",
				StorageAccountName: "",
				Location:           "",
			},
			expectError: true,
			errorMsg:    "subscription_id is required",
		},
		{
			name: "valid config with optional fields",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "12345678-1234-1234-1234-123456789012",
				ResourceGroupName:  "test-rg",
				StorageAccountName: "teststorageaccount",
				Location:           "East US",
				AccountTier:        "Standard",
				ReplicationType:    "LRS",
				AccessTier:         "Hot",
				Tags:               map[string]string{"env": "test"},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.config.Validate()
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestResourceGroupConfigValidate tests the Validate method of ResourceGroupConfig
func TestResourceGroupConfigValidate(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name        string
		config      azurehelper.ResourceGroupConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid complete config",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "12345678-1234-1234-1234-123456789012",
				ResourceGroupName: "test-rg",
				Location:          "eastus",
			},
			expectError: false,
		},
		{
			name: "missing subscription ID",
			config: azurehelper.ResourceGroupConfig{
				ResourceGroupName: "test-rg",
				Location:          "eastus",
			},
			expectError: true,
			errorMsg:    "subscription_id is required",
		},
		{
			name: "missing resource group name",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID: "12345678-1234-1234-1234-123456789012",
				Location:       "eastus",
			},
			expectError: true,
			errorMsg:    "resource_group_name is required",
		},
		{
			name: "missing location",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "12345678-1234-1234-1234-123456789012",
				ResourceGroupName: "test-rg",
			},
			expectError: true,
			errorMsg:    "location is required",
		},
		{
			name: "empty strings should fail validation",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "",
				ResourceGroupName: "",
				Location:          "",
			},
			expectError: true,
			errorMsg:    "subscription_id is required",
		},
		{
			name: "valid config with tags",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "12345678-1234-1234-1234-123456789012",
				ResourceGroupName: "test-rg",
				Location:          "eastus",
				Tags: map[string]string{
					"Environment": "test",
					"Project":     "terragrunt",
				},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.config.Validate()
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIsNotFoundError tests the IsNotFoundError utility function
func TestIsNotFoundError(t *testing.T) {
	t.Parallel()

	//nolint: govet
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.Errorf("some error"),
			expected: false,
		},
		{
			name: "Azure 404 error",
			err: &azcore.ResponseError{
				StatusCode: 404,
				ErrorCode:  "NotFound",
			},
			expected: true,
		},
		{
			name: "Azure 500 error",
			err: &azcore.ResponseError{
				StatusCode: 500,
				ErrorCode:  "InternalError",
			},
			expected: false,
		},
		{
			name: "Azure 403 error",
			err: &azcore.ResponseError{
				StatusCode: 403,
				ErrorCode:  "Forbidden",
			},
			expected: false,
		},
		{
			name: "Azure 400 error",
			err: &azcore.ResponseError{
				StatusCode: 400,
				ErrorCode:  "BadRequest",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurehelper.IsNotFoundError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAccessTierConstants tests that access tier constants are defined correctly
func TestAccessTierConstants(t *testing.T) {
	t.Parallel()

	// Test that the constants have the expected values
	assert.Equal(t, "Hot", azurehelper.AccessTierHot)
	assert.Equal(t, "Cool", azurehelper.AccessTierCool)
	assert.Equal(t, "Premium", azurehelper.AccessTierPremium)

	// Test that constants can be used in comparisons
	assert.NotEqual(t, azurehelper.AccessTierHot, azurehelper.AccessTierCool)
	assert.NotEqual(t, azurehelper.AccessTierHot, azurehelper.AccessTierPremium)
	assert.NotEqual(t, azurehelper.AccessTierCool, azurehelper.AccessTierPremium)
}

// TestErrorInterfaceImplementation tests that our error types implement the error interface
func TestErrorInterfaceImplementation(t *testing.T) {
	t.Parallel()

	// Test AzureResponseError implements error interface
	var err error = &azurehelper.AzureResponseError{
		StatusCode: 404,
		ErrorCode:  "NotFound",
		Message:    "Resource not found",
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "NotFound")
	assert.Contains(t, err.Error(), "Resource not found")
}
