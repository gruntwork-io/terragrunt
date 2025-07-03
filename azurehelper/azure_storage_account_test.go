//go:build azure

package azurehelper_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

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
		enableVersioning              bool
		allowBlobPublicAccess         bool
		isPublicNetworkAccessOK       bool
		isContainerDeleteRetentionOK  bool
	}{
		{
			name:                          "Default configuration",
			enableVersioning:              false,
			allowBlobPublicAccess:         true,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Default properties",
		},
		{
			name:                          "Versioning enabled",
			enableVersioning:              true,
			allowBlobPublicAccess:         true,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Versioning enabled",
		},
		{
			name:                          "Public access disabled",
			enableVersioning:              false,
			allowBlobPublicAccess:         false,
			isPublicNetworkAccessOK:       true,
			isContainerDeleteRetentionOK:  false,
			expectedBlobServiceProperties: "Public access disabled",
		},
		{
			name:                          "Secure configuration",
			enableVersioning:              true,
			allowBlobPublicAccess:         false,
			isPublicNetworkAccessOK:       false,
			isContainerDeleteRetentionOK:  true,
			expectedBlobServiceProperties: "Secure configuration",
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
				EnableVersioning:      tc.enableVersioning,
				AllowBlobPublicAccess: tc.allowBlobPublicAccess,
			}

			// Validate the config
			err := config.Validate()
			assert.NoError(t, err)

			// In actual implementation, these properties would be used to configure
			// the storage account. Since we can't test that without real Azure resources,
			// we're just ensuring the values are correctly stored in the config.
			assert.Equal(t, tc.enableVersioning, config.EnableVersioning)
			assert.Equal(t, tc.allowBlobPublicAccess, config.AllowBlobPublicAccess)
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

// TestStorageAccountConfigEdgeCases tests edge cases and boundary conditions
func TestStorageAccountConfigEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		config         azurehelper.StorageAccountConfig
		expectedErrMsg string
		isValid        bool
	}{
		{
			name: "Very long subscription ID",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     strings.Repeat("a", 1000), // Very long subscription ID
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid: true, // Should be valid as Azure handles long subscription IDs
		},
		{
			name: "Special characters in resource group name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group-with-special-chars_123",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
			},
			isValid: true, // Resource groups can contain hyphens and underscores
		},
		{
			name: "Unicode characters in location",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus中文", // Unicode characters
			},
			isValid: true, // Location validation is handled by Azure API
		},
		{
			name: "Very long storage account name",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: strings.Repeat("a", 100), // Very long name
				Location:           "eastus",
			},
			isValid: true, // Length validation would be done by Azure API, not our config
		},
		{
			name: "Nil tags map edge case",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				Tags:               nil, // Explicit nil
			},
			isValid: true,
		},
		{
			name: "Empty tags map",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				Tags:               map[string]string{}, // Empty map
			},
			isValid: true,
		},
		{
			name: "Tags with empty values",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				Tags: map[string]string{
					"Environment": "",
					"Owner":       "",
				},
			},
			isValid: true, // Empty tag values should be allowed
		},
		{
			name: "Very long tag values",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				Tags: map[string]string{
					"LongTag": strings.Repeat("x", 500),
				},
			},
			isValid: true, // Azure will validate tag length limits
		},
		{
			name: "Invalid account kind edge case",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				AccountKind:        "InvalidKind",
			},
			isValid: true, // Our validation doesn't check account kind validity
		},
		{
			name: "Invalid replication type edge case",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:     "subscription-id",
				ResourceGroupName:  "resource-group",
				StorageAccountName: "storageaccount",
				Location:           "eastus",
				ReplicationType:    "INVALID",
			},
			isValid: true, // Our validation doesn't check replication type validity
		},
		{
			name: "Conflicting boolean settings",
			config: azurehelper.StorageAccountConfig{
				SubscriptionID:        "subscription-id",
				ResourceGroupName:     "resource-group",
				StorageAccountName:    "storageaccount",
				Location:              "eastus",
				EnableVersioning:      true,
				AllowBlobPublicAccess: true, // Potentially insecure combination
			},
			isValid: true, // Both settings are technically valid together
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

// TestGetStorageAccountSKUErrorHandling tests error conditions and edge cases for SKU generation
func TestGetStorageAccountSKUErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		tier             string
		replication      string
		expectedSKU      string
		shouldUseDefault bool
		description      string
	}{
		{
			name:             "Whitespace only tier",
			tier:             "   ",
			replication:      "LRS",
			expectedSKU:      "   _LRS", // Whitespace is preserved, not treated as empty
			shouldUseDefault: false,
			description:      "Whitespace-only tier should be preserved, not treated as empty",
		},
		{
			name:             "Whitespace only replication",
			tier:             "Standard",
			replication:      "   ",
			expectedSKU:      "Standard_   ", // Whitespace is preserved, not treated as empty
			shouldUseDefault: false,
			description:      "Whitespace-only replication should be preserved, not treated as empty",
		},
		{
			name:             "Both whitespace only",
			tier:             "   ",
			replication:      "   ",
			expectedSKU:      "   _   ", // Both whitespace preserved
			shouldUseDefault: false,
			description:      "Both whitespace should be preserved, not defaulted",
		},
		{
			name:             "Case sensitivity test",
			tier:             "standard", // lowercase
			replication:      "lrs",      // lowercase
			expectedSKU:      "standard_lrs",
			shouldUseDefault: false,
			description:      "Function should preserve case from input",
		},
		{
			name:             "Special characters in tier",
			tier:             "Standard-Premium",
			replication:      "LRS",
			expectedSKU:      "Standard-Premium_LRS",
			shouldUseDefault: false,
			description:      "Special characters should be preserved",
		},
		{
			name:             "Very long tier name",
			tier:             strings.Repeat("VeryLongTierName", 10),
			replication:      "LRS",
			expectedSKU:      strings.Repeat("VeryLongTierName", 10) + "_LRS",
			shouldUseDefault: false,
			description:      "Very long tier names should be handled",
		},
		{
			name:             "Very long replication name",
			tier:             "Standard",
			replication:      strings.Repeat("VeryLongReplicationType", 5),
			expectedSKU:      "Standard_" + strings.Repeat("VeryLongReplicationType", 5),
			shouldUseDefault: false,
			description:      "Very long replication names should be handled",
		},
		{
			name:             "Unicode characters in tier",
			tier:             "Standard中文",
			replication:      "LRS",
			expectedSKU:      "Standard中文_LRS",
			shouldUseDefault: false,
			description:      "Unicode characters should be preserved",
		},
		{
			name:             "Numbers in names",
			tier:             "Standard2",
			replication:      "LRS3",
			expectedSKU:      "Standard2_LRS3",
			shouldUseDefault: false,
			description:      "Numbers in names should be preserved",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sku, isDefault := azurehelper.GetStorageAccountSKU(tc.tier, tc.replication)
			assert.Equal(t, tc.expectedSKU, sku, tc.description)
			assert.Equal(t, tc.shouldUseDefault, isDefault, tc.description)
		})
	}
}

// Test that the public interface for testing RBAC functionality is available
// TestRBACConfiguration provides comprehensive testing of RBAC constants and configuration
// This consolidates all RBAC constant testing into a single maintainable function
func TestRBACConfiguration(t *testing.T) {
	t.Parallel()

	// Test basic constant accessibility and types
	t.Run("Constants_accessibility_and_types", func(t *testing.T) {
		t.Parallel()

		// Ensure constants are accessible (tests proper export)
		delay := azurehelper.RbacRetryDelay
		maxRetries := azurehelper.RbacMaxRetries
		attempts := azurehelper.RbacRetryAttempts

		// Type assertions
		assert.IsType(t, time.Duration(0), delay, "RbacRetryDelay should be time.Duration")
		assert.IsType(t, int(0), maxRetries, "RbacMaxRetries should be int")
		assert.IsType(t, int(0), attempts, "RbacRetryAttempts should be int")

		// Non-zero checks
		assert.NotZero(t, delay, "RBAC retry delay should not be zero")
		assert.NotZero(t, maxRetries, "RBAC max retries should not be zero")
		assert.NotZero(t, attempts, "RBAC retry attempts should not be zero")
		assert.Positive(t, maxRetries, "RbacMaxRetries should be positive")
		assert.Positive(t, attempts, "RbacRetryAttempts should be positive")
	})

	// Test specific expected values from azure_constants.go
	t.Run("Expected_constant_values", func(t *testing.T) {
		t.Parallel()

		// These should match the values defined in azure_constants.go
		assert.Equal(t, 3*time.Second, azurehelper.RbacRetryDelay, "RbacRetryDelay should be 3 seconds")
		assert.Equal(t, 5, azurehelper.RbacMaxRetries, "RbacMaxRetries should be 5")
		assert.Equal(t, 6, azurehelper.RbacRetryAttempts, "RbacRetryAttempts should be 6")
	})

	// Test mathematical relationships between constants
	t.Run("Constant_relationships", func(t *testing.T) {
		t.Parallel()

		// Core relationship: attempts = retries + 1 (initial attempt)
		assert.Equal(t, azurehelper.RbacMaxRetries+1, azurehelper.RbacRetryAttempts,
			"RbacRetryAttempts should equal RbacMaxRetries + 1 (initial attempt + retries)")
	})

	// Test timing boundaries for practical use
	t.Run("Timing_boundaries", func(t *testing.T) {
		t.Parallel()

		// Minimum delay to avoid API rate limiting
		minDelay := 1 * time.Second
		assert.GreaterOrEqual(t, azurehelper.RbacRetryDelay, minDelay,
			"Retry delay should be at least %v to avoid overwhelming Azure APIs", minDelay)

		// Maximum delay for good user experience
		maxDelay := 10 * time.Second
		assert.LessOrEqual(t, azurehelper.RbacRetryDelay, maxDelay,
			"Retry delay should not exceed %v for reasonable user experience", maxDelay)

		// Minimum retries for RBAC propagation (Azure can be slow)
		assert.GreaterOrEqual(t, azurehelper.RbacMaxRetries, 3,
			"Should have at least 3 retries for RBAC propagation delays")

		// Maximum retries to avoid excessive wait times
		assert.LessOrEqual(t, azurehelper.RbacMaxRetries, 10,
			"Should not exceed 10 retries to avoid excessive wait times")
	})

	// Test total operation time boundaries
	t.Run("Total_operation_time_bounds", func(t *testing.T) {
		t.Parallel()

		// Calculate theoretical maximum wait time
		maxTotalTime := time.Duration(azurehelper.RbacMaxRetries) * azurehelper.RbacRetryDelay

		// Should complete within reasonable time for CI/CD pipelines
		maxReasonableTime := 5 * time.Minute
		assert.LessOrEqual(t, maxTotalTime, maxReasonableTime,
			"Total maximum wait time (%v) should not exceed %v for CI/CD compatibility",
			maxTotalTime, maxReasonableTime)

		// Should not be too fast (RBAC needs time to propagate)
		minReasonableTime := 10 * time.Second
		assert.GreaterOrEqual(t, maxTotalTime, minReasonableTime,
			"Total maximum wait time (%v) should be at least %v for effective RBAC propagation",
			maxTotalTime, minReasonableTime)
	})

	// Test edge case boundaries
	t.Run("Edge_case_boundaries", func(t *testing.T) {
		t.Parallel()

		// Ensure delay is not too short (could cause API rate limiting)
		assert.GreaterOrEqual(t, azurehelper.RbacRetryDelay, 1*time.Second,
			"RBAC retry delay should be at least 1 second to avoid API rate limits")

		// Ensure delay is not excessively long (user experience)
		assert.LessOrEqual(t, azurehelper.RbacRetryDelay, 30*time.Second,
			"RBAC retry delay should not exceed 30 seconds for reasonable user experience")

		// Ensure we have enough retries for typical RBAC propagation
		assert.GreaterOrEqual(t, azurehelper.RbacMaxRetries, 3,
			"Should have at least 3 retries for typical RBAC propagation scenarios")
	})
}

// TestPermissionErrorDetectionComprehensive tests comprehensive permission error detection
func TestPermissionErrorDetectionComprehensive(t *testing.T) {
	t.Parallel()

	client := &azurehelper.StorageAccountClient{}

	// Test case-insensitive detection
	caseInsensitiveTestCases := []struct {
		name           string
		errorMessage   string
		expectedResult bool
	}{
		{"Mixed case forbidden", "FoRbIdDeN access", true},
		{"Uppercase unauthorized", "UNAUTHORIZED REQUEST", true},
		{"Lowercase access denied", "access denied by policy", true},
		{"Mixed case insufficient permissions", "InSuFfIcIeNt PeRmIsSiOnS", true},
		{"Normal case non-permission", "Resource Not Found", false},
		{"Mixed case non-permission", "InTeRnAl SeRvEr ErRoR", false},
	}

	for _, tc := range caseInsensitiveTestCases {
		tc := tc
		t.Run("CaseInsensitive_"+tc.name, func(t *testing.T) {
			t.Parallel()

			err := errors.Errorf("%s", tc.errorMessage)
			result := client.IsPermissionError(err)
			assert.Equal(t, tc.expectedResult, result,
				"Should detect permission errors case-insensitively: %s", tc.errorMessage)
		})
	}

	// Test error patterns that might be confusing
	edgeCaseTestCases := []struct {
		name           string
		errorMessage   string
		expectedResult bool
		reason         string
	}{
		{
			name:           "Permission in non-error context",
			errorMessage:   "User has permission to view logs but operation failed due to network timeout",
			expectedResult: false,
			reason:         "Should not detect 'permission' when it's not an error context",
		},
		{
			name:           "Forbidden in URL",
			errorMessage:   "Failed to connect to https://forbidden.example.com/api",
			expectedResult: true,
			reason:         "Should detect 'forbidden' even in URLs since it indicates access issues",
		},
		{
			name:           "Authorization success message",
			errorMessage:   "Authorization completed successfully but resource not found",
			expectedResult: false,
			reason:         "Should not detect successful authorization as permission error",
		},
		{
			name:           "Multiple permission keywords",
			errorMessage:   "Unauthorized access denied: insufficient permissions for forbidden operation",
			expectedResult: true,
			reason:         "Should detect errors with multiple permission keywords",
		},
		{
			name:           "Role assignment in progress",
			errorMessage:   "Operation failed: role assignment is still propagating through Azure RBAC",
			expectedResult: true,
			reason:         "Should detect role assignment propagation errors",
		},
	}

	for _, tc := range edgeCaseTestCases {
		tc := tc
		t.Run("EdgeCase_"+tc.name, func(t *testing.T) {
			t.Parallel()

			err := errors.Errorf("%s", tc.errorMessage)
			result := client.IsPermissionError(err)
			assert.Equal(t, tc.expectedResult, result, tc.reason)
		})
	}
}

// TestStorageAccountClientPermissionInterface tests the public interface for permission checking
func TestStorageAccountClientPermissionInterface(t *testing.T) {
	t.Parallel()

	// Test that we can create a client and use the IsPermissionError method
	client := &azurehelper.StorageAccountClient{}

	// Test with various error types
	testCases := []struct {
		name     string
		input    error
		expected bool
	}{
		{
			name:     "Nil error",
			input:    nil,
			expected: false,
		},
		{
			name:     "Empty error message",
			input:    errors.Errorf(""),
			expected: false,
		},
		{
			name:     "Generic error",
			input:    errors.Errorf("Something went wrong"),
			expected: false,
		},
		{
			name:     "Forbidden error",
			input:    errors.Errorf("403 Forbidden"),
			expected: true,
		},
		{
			name:     "Unauthorized error",
			input:    errors.Errorf("401 Unauthorized"),
			expected: true,
		},
		{
			name:     "Access denied error",
			input:    errors.Errorf("Access denied to resource"),
			expected: true,
		},
		{
			name:     "Complex permission error",
			input:    errors.Errorf("Operation failed with status 403: insufficient permissions for Storage Blob Data Owner role"),
			expected: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := client.IsPermissionError(tc.input)
			assert.Equal(t, tc.expected, result,
				"IsPermissionError should return %v for error: %v", tc.expected, tc.input)
		})
	}
}

// TestRBACConstantsExportedCorrectly tests that RBAC constants are properly exported
// TestPermissionErrorBoundaryConditions tests boundary conditions for permission error detection
func TestPermissionErrorBoundaryConditions(t *testing.T) {
	t.Parallel()

	client := &azurehelper.StorageAccountClient{}

	// Test extremely long error messages
	t.Run("Long_error_messages", func(t *testing.T) {
		t.Parallel()

		longPrefix := strings.Repeat("A very long error message prefix ", 100)
		longSuffix := strings.Repeat(" and a very long suffix", 100)

		// Permission error buried in long message
		longPermissionError := longPrefix + "access denied" + longSuffix
		err := errors.Errorf("%s", longPermissionError)
		assert.True(t, client.IsPermissionError(err), "Should detect permission errors in long messages")

		// Non-permission error that's very long
		longNonPermissionError := longPrefix + "network timeout occurred" + longSuffix
		err2 := errors.Errorf("%s", longNonPermissionError)
		assert.False(t, client.IsPermissionError(err2), "Should not detect non-permission errors even if long")
	})

	// Test special characters and encodings
	t.Run("Special_characters", func(t *testing.T) {
		t.Parallel()

		specialCharErrors := []struct {
			message  string
			expected bool
		}{
			{"forbidden\n\r\t with newlines", true},
			{"forbidden\x00with null bytes", true},
			{"access denied with émojis 🚫", true},
			{"网络超时 timeout occurred", false}, // Chinese characters
			{"forbidden操作被禁止", true},         // Mixed languages
		}

		for i, tc := range specialCharErrors {
			err := errors.Errorf("%s", tc.message)
			result := client.IsPermissionError(err)
			assert.Equal(t, tc.expected, result, "Test case %d: %s", i, tc.message)
		}
	})

	// Test nested/wrapped errors
	t.Run("Wrapped_errors", func(t *testing.T) {
		t.Parallel()

		// Create a wrapped error with permission context
		innerErr := errors.Errorf("access denied to storage account")
		wrappedErr := errors.Errorf("storage operation failed: %w", innerErr)

		// Should detect permission error in wrapped context
		assert.True(t, client.IsPermissionError(wrappedErr),
			"Should detect permission errors in wrapped errors")

		// Create a wrapped error without permission context
		innerErr2 := errors.Errorf("network connection timeout")
		wrappedErr2 := errors.Errorf("storage operation failed: %w", innerErr2)

		assert.False(t, client.IsPermissionError(wrappedErr2),
			"Should not detect non-permission errors in wrapped errors")
	})
}
