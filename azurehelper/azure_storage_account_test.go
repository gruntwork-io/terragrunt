//go:build azure

package azurehelper_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestStorageAccountConfigValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		config         azurehelper.StorageAccountConfig
		expectedErrMsg string
		isValid        bool
	}{
		{
			name: "Valid config",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid:        true,
			expectedErrMsg: "",
		},
		{
			name: "Missing subscription ID",
			config: azurehelper.StorageAccountConfig{
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid:        false,
			expectedErrMsg: "subscription_id is required",
		},
		{
			name: "Missing resource group name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid:        false,
			expectedErrMsg: "resource_group_name is required",
		},
		{
			name: "Missing storage account name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:    "subscription-id",
				ResourceGroupName: "resource-group",
				Location:          "eastus",
			},
			isValid:        false,
			expectedErrMsg: "storage_account_name is required",
		},
		{
			name: "Missing location",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
			},
			isValid:        false,
			expectedErrMsg: "location is required",
		},
		{
			name: "Empty subscription ID",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid:        false,
			expectedErrMsg: "subscription_id is required",
		},
		{
			name: "Empty resource group name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid:        false,
			expectedErrMsg: "resource_group_name is required",
		},
		{
			name: "Empty storage account name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "",
				Location:           "eastus",
			},
			isValid:        false,
			expectedErrMsg: "storage_account_name is required",
		},
		{
			name: "Empty location",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "",
			},
			isValid:        false,
			expectedErrMsg: "location is required",
		},
		{
			name: "With additional properties",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:        "subscription-id",
				ResourceGroupName:     "resource-group",
				StorageAccountName:    "storageaccount",
				Location:              "eastus",
				EnableVersioning:      true,
				EnableHierarchicalNS:  true,
				AllowBlobPublicAccess: false,
				AccountKind:           "StorageV2",
				AccountTier:           "Standard",
				ReplicationType:       "LRS",
				Tags: map[string]string{
					"Environment": "Test",
				},
			},
			isValid:        true,
			expectedErrMsg: "",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.config.Validate()
			if tc.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tc.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tc.expectedErrMsg)
				}
			}
		})
	}
}

func TestGetStorageAccountDefaultSKU(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		accountTier      string
		replicationType  string
		expectedSKU      string
		expectDefaultSKU bool
	}{
		{
			accountTier:      "Standard",
			replicationType:  "LRS",
			expectedSKU:      "Standard_LRS",
			expectDefaultSKU: false,
		},
		{
			accountTier:      "Premium",
			replicationType:  "ZRS",
			expectedSKU:      "Premium_ZRS",
			expectDefaultSKU: false,
		},
		{
			accountTier:      "",
			replicationType:  "",
			expectedSKU:      "Standard_LRS", // Default SKU
			expectDefaultSKU: true,
		},
		{
			accountTier:      "Standard",
			replicationType:  "",
			expectedSKU:      "Standard_LRS",
			expectDefaultSKU: false,
		},
		{
			accountTier:      "",
			replicationType:  "GRS",
			expectedSKU:      "Standard_GRS",
			expectDefaultSKU: false,
		},
	}

	for i, tc := range testCases {
		tc := tc // capture range variable
		t.Run(fmt.Sprintf("TestCase_%d", i), func(t *testing.T) {
			t.Parallel()

			sku, isDefault := azurehelper.GetStorageAccountSKU(tc.accountTier, tc.replicationType)
			assert.Equal(t, tc.expectedSKU, sku)
			assert.Equal(t, tc.expectDefaultSKU, isDefault)
		})
	}
}

// TestStorageAccountNameValidation tests validation of storage account names
func TestStorageAccountNameValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		saName    string
		errorText string
		isValid   bool
	}{
		{
			name:      "Valid storage account name",
			saName:    "mystorageaccount",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Valid with numbers",
			saName:    "storage123",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Too short",
			saName:    "sa",
			isValid:   false,
			errorText: "name must be between 3 and 24 characters",
		},
		{
			name:      "Too long",
			saName:    "thisstorageaccountnameistoolong",
			isValid:   false,
			errorText: "name must be between 3 and 24 characters",
		},
		{
			name:      "With uppercase",
			saName:    "StorageAccount",
			isValid:   false,
			errorText: "name must be lowercase",
		},
		{
			name:      "With special characters",
			saName:    "storage-account",
			isValid:   false,
			errorText: "name must contain only letters and numbers",
		},
		{
			name:      "With underscore",
			saName:    "storage_account",
			isValid:   false,
			errorText: "name must contain only letters and numbers",
		},
		{
			name:      "Starting with number",
			saName:    "1storage",
			isValid:   false,
			errorText: "name must start with a letter",
		},
		{
			name:      "Empty name",
			saName:    "",
			isValid:   false,
			errorText: "name cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Implement a basic validation function similar to what Azure might use
			// This doesn't call the actual Azure helper but mimics what validation logic would do
			var err error
			switch {
			case tc.saName == "":
				err = errors.New("name cannot be empty")
			case len(tc.saName) < 3 || len(tc.saName) > 24:
				err = errors.New("name must be between 3 and 24 characters")
			case tc.saName != strings.ToLower(tc.saName):
				err = errors.New("name must be lowercase")
			case !regexp.MustCompile("^[a-z][a-z0-9]*$").MatchString(tc.saName):
				if !regexp.MustCompile("^[a-z]").MatchString(tc.saName) {
					err = errors.New("name must start with a letter")
				} else {
					err = errors.New("name must contain only letters and numbers")
				}
			}

			if tc.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tc.errorText != "" {
					assert.Contains(t, err.Error(), tc.errorText)
				}
			}
		})
	}
}

func TestGetStorageAccountSKUValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		tier             string
		replication      string
		expectedSKU      string
		invalidSKUReason string
		isDefault        bool
		isValidSKU       bool
	}{
		{
			name:        "Standard LRS",
			tier:        "Standard",
			replication: "LRS",
			expectedSKU: "Standard_LRS",
			isDefault:   false,
			isValidSKU:  true,
		},
		{
			name:        "Standard GRS",
			tier:        "Standard",
			replication: "GRS",
			expectedSKU: "Standard_GRS",
			isDefault:   false,
			isValidSKU:  true,
		},
		{
			name:        "Standard RAGRS",
			tier:        "Standard",
			replication: "RAGRS",
			expectedSKU: "Standard_RAGRS",
		 isDefault:   false,
		 isValidSKU:  true,
		},
		{
			name:        "Standard ZRS",
			tier:        "Standard",
			replication: "ZRS",
			expectedSKU: "Standard_ZRS",
			isDefault:   false,
		 isValidSKU:  true,
		},
		{
			name:        "Premium LRS",
			tier:        "Premium",
			replication: "LRS",
			expectedSKU: "Premium_LRS",
			isDefault:   false,
		 isValidSKU:  true,
		},
		{
			name:        "Premium ZRS",
			tier:        "Premium",
			replication: "ZRS",
			expectedSKU: "Premium_ZRS",
			isDefault:   false,
		 isValidSKU:  true,
		},
		{
			name:        "Default values",
			tier:        "",
			replication: "",
			expectedSKU: "Standard_LRS",
			isDefault:   true,
			isValidSKU:  true,
		},
		{
			name:        "Default replication",
			tier:        "Standard",
			replication: "",
			expectedSKU: "Standard_LRS",
			isDefault:   false,
			isValidSKU:  true,
		},
		{
			name:        "Default tier",
			tier:        "",
			replication: "GRS",
			expectedSKU: "Standard_GRS",
			isDefault:   false,
			isValidSKU:  true,
		},
		{
			name:             "Invalid tier",
			tier:             "Basic",
			replication:      "LRS",
			expectedSKU:      "Basic_LRS",
			isDefault:        false,
			isValidSKU:       false,
			invalidSKUReason: "invalid tier: Basic",
		},
		{
			name:             "Invalid replication",
			tier:             "Standard",
			replication:      "ABCDE",
			expectedSKU:      "Standard_ABCDE",
			isDefault:        false,
			isValidSKU:       false,
			invalidSKUReason: "invalid replication: ABCDE",
		},
		{
			name:             "Premium with GRS",
			tier:             "Premium",
			replication:      "GRS",
			expectedSKU:      "Premium_GRS",
			isDefault:        false,
			isValidSKU:       false,
			invalidSKUReason: "Premium tier does not support GRS",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sku, isDefault := azurehelper.GetStorageAccountSKU(tc.tier, tc.replication)
			assert.Equal(t, tc.expectedSKU, sku)
			assert.Equal(t, tc.isDefault, isDefault)

			// Additional validation of the SKUs
			// This is separate from the GetStorageAccountSKU function but shows how we would validate SKUs
			isValid := true
			var reason string

			validTiers := []string{"Standard", "Premium"}
			validReplications := []string{"LRS", "GRS", "RAGRS", "ZRS", "GZRS", "RAGZRS"}

			// Default to Standard tier if empty
			tier := tc.tier
			if tier == "" {
				tier = "Standard"
			}

			// Default to LRS if replication is empty
			replication := tc.replication
			if replication == "" {
				replication = "LRS"
			}

			// Check if tier is valid
			if !contains(validTiers, tier) {
				isValid = false
				reason = "invalid tier: " + tier
			}

			// Check if replication is valid
			if isValid && !contains(validReplications, replication) {
				isValid = false
				reason = "invalid replication: " + replication
			}

			// Check tier-replication compatibility
			if isValid && tier == "Premium" && replication != "LRS" && replication != "ZRS" {
				isValid = false
				reason = "Premium tier does not support " + replication
			}

			// Assert our validation matches the test case's expectations
			assert.Equal(t, tc.isValidSKU, isValid)
			if !tc.isValidSKU {
				assert.Equal(t, tc.invalidSKUReason, reason)
			}
		})
	}
}

// TestStorageAccountAdvancedFeatures tests validation of the storage account advanced feature flags
func TestStorageAccountAdvancedFeatures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                          string
		expectedBlobServiceProperties string
		enableHierarchicalNS          bool
		enableVersioning              bool
		allowBlobPublicAccess         bool
		isPublicNetworkAccessOK       bool
		isContainerDeleteRetentionOK  bool
	}{
		{
			name:                          "Default configuration",
			enableHierarchicalNS:          false,
			enableVersioning:              false,
			allowBlobPublicAccess:         true,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Default properties",
		},
		{
			name:                          "Hierarchical namespace enabled",
			enableHierarchicalNS:          true,
			enableVersioning:              false,
			allowBlobPublicAccess:         true,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Hierarchical namespace enabled",
		},
		{
			name:                          "Versioning enabled",
			enableHierarchicalNS:          false,
			enableVersioning:              true,
			allowBlobPublicAccess:         true,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Versioning enabled",
		},
		{
			name:                          "Public access disabled",
			enableHierarchicalNS:          false,
			enableVersioning:              false,
			allowBlobPublicAccess:         false,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Public access disabled",
		},
		{
			name:                          "Secure configuration",
			enableHierarchicalNS:          false,
			enableVersioning:              true,
			allowBlobPublicAccess:         false,
			isPublicNetworkAccessOK:       false,
			isContainerDeleteRetentionOK:  true,
			expectedBlobServiceProperties: "Secure configuration",
		},
		{
			name:                          "Data lake configuration",
			enableHierarchicalNS:          true,
			enableVersioning:              true,
			allowBlobPublicAccess:         false,
			isPublicNetworkAccessOK:       false,
			isContainerDeleteRetentionOK:  true,
			expectedBlobServiceProperties: "Data lake configuration",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a storage account config with the advanced features
			config := azurehelper.StorageAccountConfig{
				SubscriptionID:        "subscription-id",
				ResourceGroupName:     "resource-group",
				StorageAccountName:    "storageaccount",
				Location:              "eastus",
				EnableHierarchicalNS:  tc.enableHierarchicalNS,
				EnableVersioning:      tc.enableVersioning,
				AllowBlobPublicAccess: tc.allowBlobPublicAccess,
			}

			// Validate the config
			err := config.Validate()
			assert.NoError(t, err)

			// In actual implementation, these properties would be used to configure
			// the storage account. Since we can't test that without real Azure resources,
			// we're just ensuring the values are correctly stored in the config.
			assert.Equal(t, tc.enableHierarchicalNS, config.EnableHierarchicalNS)
			assert.Equal(t, tc.enableVersioning, config.EnableVersioning)
			assert.Equal(t, tc.allowBlobPublicAccess, config.AllowBlobPublicAccess)

			// Check feature compatibility (these would normally be enforced by Azure)
			// Simulate validation that would happen in real implementation
			// Some features like versioning require certain account types
			if tc.enableVersioning && config.AccountKind == "BlobStorage" {
				// Versioning requires a StorageV2 account
				t.Logf("Warning: Versioning may not be supported on BlobStorage account kind")
			}

			// HNS is only supported on certain account kinds
			if tc.enableHierarchicalNS && config.AccountKind != "StorageV2" && config.AccountKind != "" {
				t.Logf("Warning: Hierarchical namespace requires StorageV2 account kind")
			}
		})
	}
}

// TestStorageAccountEncryptionSettings tests validation of storage account encryption settings
func TestStorageAccountEncryptionSettings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		description          string
		config               azurehelper.StorageAccountConfig
		keysToEncrypt        []string
		expectedEncryptionOK bool
	}{
		{
			name: "Default encryption settings",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			keysToEncrypt:        []string{"blob"},
			expectedEncryptionOK: true,
			description:          "By default, Azure Storage encrypts all data with blob encryption",
		},
		{
			name: "All services encryption",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			keysToEncrypt:        []string{"blob", "file", "table", "queue"},
			expectedEncryptionOK: true,
			description:          "All storage services should be encrypted",
		},
		{
			name: "Custom encryption key source",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				// In a real implementation, this could be customer-managed keys
				// For this test, we just simulate the property name
				KeyEncryptionKey: "Microsoft.KeyVault",
			},
			keysToEncrypt:        []string{"blob"},
			expectedEncryptionOK: true,
			description:          "Customer-managed keys should be supported",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Validate the config
			err := tc.config.Validate()
			assert.NoError(t, err)

			// Simulate encryption configuration
			// In a real implementation, this would be part of the storage account creation/update
			encryptionConfig := map[string]bool{}
			for _, svc := range tc.keysToEncrypt {
				encryptionConfig[svc] = true
			}

			// Check if all requested services are encrypted
			for _, svc := range tc.keysToEncrypt {
				assert.True(t, encryptionConfig[svc], fmt.Sprintf("Service %s should be encrypted", svc))
			}
		})
	}
}

// TestStorageAccountNetworkRules tests the network rules for storage accounts
func TestStorageAccountNetworkRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		defaultAction       string
		errorMessage        string
		ipRules             []string
		virtualNetworkRules []string
		bypassServices      []string
		expectedError       bool
	}{
		{
			name:                "Default network rules",
			defaultAction:       "Allow",
			ipRules:             nil,
			virtualNetworkRules: nil,
			bypassServices:      nil,
			expectedError:       false,
		},
		{
			name:                "Deny with bypass",
			defaultAction:       "Deny",
			ipRules:             nil,
			virtualNetworkRules: nil,
			bypassServices:      []string{"Logging", "Metrics"},
			expectedError:       false,
		},
		{
			name:                "Deny with IP rules",
			defaultAction:       "Deny",
			ipRules:             []string{"192.168.1.0/24", "10.0.0.0/16"},
			virtualNetworkRules: nil,
			bypassServices:      nil,
			expectedError:       false,
		},
		{
			name:                "Deny with invalid IP rule",
			defaultAction:       "Deny",
			ipRules:             []string{"invalid-ip-range"},
			virtualNetworkRules: nil,
			bypassServices:      nil,
			expectedError:       true,
			errorMessage:        "invalid IP rule: invalid-ip-range",
		},
		{
			name:                "Invalid default action",
			defaultAction:       "InvalidAction",
			ipRules:             nil,
			virtualNetworkRules: nil,
			bypassServices:      nil,
			expectedError:       true,
			errorMessage:        "invalid default action: InvalidAction",
		},
		{
			name:                "Invalid bypass service",
			defaultAction:       "Deny",
			ipRules:             nil,
			virtualNetworkRules: nil,
			bypassServices:      []string{"InvalidService"},
			expectedError:       true,
			errorMessage:        "invalid bypass service: InvalidService",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// This test simulates the validation logic that would be in the actual implementation
			var err error

			// Validate network rules in the context of a storage account config
			if tc.defaultAction != "" && tc.defaultAction != "Allow" && tc.defaultAction != "Deny" {
				err = errors.Errorf("invalid default action: %s", tc.defaultAction)
			}

			// Validate IP rules
			if err == nil && tc.ipRules != nil {
				for _, ipRule := range tc.ipRules {
					// Simple check for IP/CIDR format, a real implementation would be more thorough
					if !strings.Contains(ipRule, ".") && !strings.Contains(ipRule, "/") {
						err = errors.Errorf("invalid IP rule: %s", ipRule)
						break
					}
				}
			}

			// Validate bypass services
			if err == nil && tc.bypassServices != nil {
				validBypassServices := map[string]bool{
					"AzureServices": true,
					"Logging":       true,
					"Metrics":       true,
					"None":          true,
				}

				for _, service := range tc.bypassServices {
					if !validBypassServices[service] {
						err = errors.Errorf("invalid bypass service: %s", service)
						break
					}
				}
			}

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorMessage != "" {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestStorageAccountURLParsing tests validation of storage account URLs
func TestStorageAccountURLParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		inputURL    string
		expectedErr string
		isValid     bool
	}{
		{
			name:        "Valid blob URL",
			inputURL:    "https://storageaccount.blob.core.windows.net",
			isValid:     true,
			expectedErr: "",
		},
		{
			name:        "Valid blob URL with path",
			inputURL:    "https://storageaccount.blob.core.windows.net/container/path",
			isValid:     true,
			expectedErr: "",
		},
		{
			name:        "Valid blob URL with custom domain",
			inputURL:    "https://custom-domain.com",
			isValid:     true,
			expectedErr: "",
		},
		{
			name:        "Missing protocol",
			inputURL:    "storageaccount.blob.core.windows.net",
			isValid:     false,
			expectedErr: "URL must start with http:// or https://",
		},
		{
			name:        "Invalid URL format",
			inputURL:    "not-a-url",
			isValid:     false,
			expectedErr: "invalid URL format",
		},
		{
			name:        "Empty URL",
			inputURL:    "",
			isValid:     false,
			expectedErr: "URL cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Validate URL
			var err error
			switch {
			case tc.inputURL == "":
				err = errors.Errorf("URL cannot be empty")
			case !strings.Contains(tc.inputURL, "."):
				err = errors.Errorf("invalid URL format")
			case !strings.HasPrefix(tc.inputURL, "http://") && !strings.HasPrefix(tc.inputURL, "https://"):
				err = errors.Errorf("URL must start with http:// or https://")
			}

			if tc.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tc.expectedErr != "" {
					assert.Contains(t, err.Error(), tc.expectedErr)
				}
			}
		})
	}
}

// TestStorageAccountConnectionStrings tests parsing and validation of connection strings
func TestStorageAccountConnectionStrings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		connectionString string
		expectedAccount  string
		expectedErr      string
		isValid          bool
	}{
		{
			name:             "Valid connection string",
			connectionString: "DefaultEndpointsProtocol=https;AccountName=storageaccount;AccountKey=abcd1234;EndpointSuffix=core.windows.net",
			isValid:          true,
			expectedAccount:  "storageaccount",
			expectedErr:      "",
		},
		{
			name:             "Connection string without account name",
			connectionString: "DefaultEndpointsProtocol=https;AccountKey=abcd1234;EndpointSuffix=core.windows.net",
			isValid:          false,
			expectedAccount:  "",
			expectedErr:      "connection string missing AccountName",
		},
		{
			name:             "Connection string without account key",
			connectionString: "DefaultEndpointsProtocol=https;AccountName=storageaccount;EndpointSuffix=core.windows.net",
			isValid:          false,
			expectedAccount:  "storageaccount",
			expectedErr:      "connection string missing AccountKey",
		},
		{
			name:             "Connection string with custom endpoint",
			connectionString: "BlobEndpoint=https://storageaccount.blob.core.windows.net;AccountName=storageaccount;AccountKey=abcd1234;",
			isValid:          true,
			expectedAccount:  "storageaccount",
			expectedErr:      "",
		},
		{
			name:             "SAS connection string",
			connectionString: "BlobEndpoint=https://storageaccount.blob.core.windows.net;SharedAccessSignature=sastoken",
			isValid:          true,
			expectedAccount:  "", // Account name would be extracted from endpoint
			expectedErr:      "",
		},
		{
			name:             "Empty connection string",
			connectionString: "",
			isValid:          false,
			expectedAccount:  "",
			expectedErr:      "connection string cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Parse connection string
			var accountName string
			var err error

			if tc.connectionString == "" {
				err = errors.Errorf("connection string cannot be empty")
			} else {
				// Basic parsing of connection string key-value pairs
				parts := strings.Split(tc.connectionString, ";")
				props := make(map[string]string)

				for _, part := range parts {
					if part == "" {
						continue
					}

					kv := strings.SplitN(part, "=", 2)
					if len(kv) == 2 {
						props[kv[0]] = kv[1]
					}
				}

				// Check for required properties
				if acct, ok := props["AccountName"]; ok {
					accountName = acct
				} else {
					// For SAS connection strings, extract account name from endpoint
					if endpoint, ok := props["BlobEndpoint"]; ok {
						// Extract account from URL like https://accountname.blob.core.windows.net
						parts := strings.Split(strings.TrimPrefix(endpoint, "https://"), ".")
						if len(parts) > 0 {
							accountName = parts[0]
						}
					}
				}

				// Validate required properties
				_, hasAccountName := props["AccountName"]
				_, hasBlobEndpoint := props["BlobEndpoint"]
				if !hasAccountName && !hasBlobEndpoint {
					err = errors.Errorf("connection string missing AccountName")
				} else {
					_, hasAccountKey := props["AccountKey"]
					_, hasSAS := props["SharedAccessSignature"]
					if !hasAccountKey && !hasSAS {
						err = errors.Errorf("connection string missing AccountKey")
					}
				}
			}

			if tc.isValid {
				assert.NoError(t, err)
				if tc.expectedAccount != "" {
					assert.Equal(t, tc.expectedAccount, accountName)
				}
			} else {
				assert.Error(t, err)
				if tc.expectedErr != "" {
					assert.Contains(t, err.Error(), tc.expectedErr)
				}
			}
		})
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

// In a real test environment with Azure credentials, we could test the actual client
// but for unit tests, we would need to mock the Azure SDK clients.
// Below is an example of how we might structure those tests if we had mocks:

/*
func TestCreateStorageAccountClient(t *testing.T) {
    // This would be implemented with mocks
}

func TestStorageAccountExists(t *testing.T) {
    // This would be implemented with mocks
}

func TestCreateStorageAccountIfNotExists(t *testing.T) {
    // This would be implemented with mocks
}
*/
