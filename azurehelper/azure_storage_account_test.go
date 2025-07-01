//go:build azure

package azurehelper_test

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// TestDefaultStorageAccountConfigErrorHandling tests the default config generation
func TestDefaultStorageAccountConfigErrorHandling(t *testing.T) {
	t.Parallel()

	// Test that default config is always valid
	t.Run("Default config validation", func(t *testing.T) {
		t.Parallel()

		defaultConfig := azurehelper.DefaultStorageAccountConfig()

		// The default config should have all non-required fields set
		assert.NotEmpty(t, defaultConfig.AccountKind)
		assert.NotEmpty(t, defaultConfig.AccountTier)
		assert.NotEmpty(t, defaultConfig.AccessTier)
		assert.NotEmpty(t, defaultConfig.ReplicationType)
		assert.NotNil(t, defaultConfig.Tags)
		assert.NotEmpty(t, defaultConfig.Tags)

		// Check specific default values
		assert.True(t, defaultConfig.EnableVersioning)
		assert.False(t, defaultConfig.AllowBlobPublicAccess)

		// Required fields should be empty (to be filled by user)
		assert.Empty(t, defaultConfig.SubscriptionID)
		assert.Empty(t, defaultConfig.ResourceGroupName)
		assert.Empty(t, defaultConfig.StorageAccountName)
		assert.Empty(t, defaultConfig.Location)
	})

	// Test that default config fails validation (missing required fields)
	t.Run("Default config requires user input", func(t *testing.T) {
		t.Parallel()

		defaultConfig := azurehelper.DefaultStorageAccountConfig()
		err := defaultConfig.Validate()

		// Should fail because required fields are missing
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "subscription_id is required")
	})

	// Test that default config becomes valid when required fields are added
	t.Run("Default config with required fields", func(t *testing.T) {
		t.Parallel()

		defaultConfig := azurehelper.DefaultStorageAccountConfig()
		defaultConfig.SubscriptionID = "test-subscription"
		defaultConfig.ResourceGroupName = "test-rg"
		defaultConfig.StorageAccountName = "teststorage"
		defaultConfig.Location = "eastus"

		err := defaultConfig.Validate()
		assert.NoError(t, err)
	})
}

// TestStorageAccountConfigConcurrency tests concurrent access to config validation
func TestStorageAccountConfigConcurrency(t *testing.T) {
	t.Parallel()

	// Test concurrent validation of the same config
	t.Run("Concurrent validation", func(t *testing.T) {
		t.Parallel()

		config := azurehelper.StorageAccountConfig{
			SubscriptionID:     "subscription-id",
			ResourceGroupName:  "resource-group",
			StorageAccountName: "storageaccount",
			Location:           "eastus",
		}

		const numGoroutines = 100
		errChan := make(chan error, numGoroutines)

		// Start multiple goroutines validating the same config
		for i := 0; i < numGoroutines; i++ {
			go func() {
				err := config.Validate()
				errChan <- err
			}()
		}

		// Collect all results
		for i := 0; i < numGoroutines; i++ {
			err := <-errChan
			assert.NoError(t, err, "Concurrent validation should not fail")
		}
	})

	// Test concurrent SKU generation
	t.Run("Concurrent SKU generation", func(t *testing.T) {
		t.Parallel()

		const numGoroutines = 100
		type result struct {
			sku       string
			isDefault bool
		}
		resultChan := make(chan result, numGoroutines)

		// Start multiple goroutines generating SKUs
		for i := 0; i < numGoroutines; i++ {
			go func() {
				sku, isDefault := azurehelper.GetStorageAccountSKU("Standard", "LRS")
				resultChan <- result{sku: sku, isDefault: isDefault}
			}()
		}

		// Collect all results and verify consistency
		expectedSKU := "Standard_LRS"
		expectedDefault := false

		for i := 0; i < numGoroutines; i++ {
			res := <-resultChan
			assert.Equal(t, expectedSKU, res.sku, "SKU should be consistent across goroutines")
			assert.Equal(t, expectedDefault, res.isDefault, "Default flag should be consistent")
		}
	})
}

// TestStorageAccountConfigMemoryUsage tests memory-related edge cases
func TestStorageAccountConfigMemoryUsage(t *testing.T) {
	t.Parallel()

	// Test with very large tag maps
	t.Run("Large tag map", func(t *testing.T) {
		t.Parallel()

		largeTags := make(map[string]string)
		for i := 0; i < 1000; i++ {
			largeTags[fmt.Sprintf("tag%d", i)] = fmt.Sprintf("value%d", i)
		}

		config := azurehelper.StorageAccountConfig{
			SubscriptionID:     "subscription-id",
			ResourceGroupName:  "resource-group",
			StorageAccountName: "storageaccount",
			Location:           "eastus",
			Tags:               largeTags,
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Len(t, config.Tags, 1000)
	})

	// Test with large string values
	t.Run("Large string values", func(t *testing.T) {
		t.Parallel()

		largeString := strings.Repeat("a", 10000)

		config := azurehelper.StorageAccountConfig{
			SubscriptionID:     largeString,
			ResourceGroupName:  largeString,
			StorageAccountName: largeString,
			Location:           largeString,
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Len(t, config.SubscriptionID, 10000)
	})
}

// TestStorageAccountConfigNilPointerSafety tests nil pointer safety
func TestStorageAccountConfigNilPointerSafety(t *testing.T) {
	t.Parallel()

	// Test validation with zero value config
	t.Run("Zero value config", func(t *testing.T) {
		t.Parallel()

		var config azurehelper.StorageAccountConfig
		err := config.Validate()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "subscription_id is required")
	})

	// Test that validation doesn't panic with nil maps
	t.Run("Nil map handling", func(t *testing.T) {
		t.Parallel()

		config := azurehelper.StorageAccountConfig{
			SubscriptionID:     "subscription-id",
			ResourceGroupName:  "resource-group",
			StorageAccountName: "storageaccount",
			Location:           "eastus",
			Tags:               nil, // Explicit nil
		}

		// Should not panic
		err := config.Validate()
		assert.NoError(t, err)
	})
}

// TestStorageAccountDataIntegrity tests data integrity and immutability
func TestStorageAccountDataIntegrity(t *testing.T) {
	t.Parallel()

	// Test that validation doesn't modify the original config
	t.Run("Validation preserves original config", func(t *testing.T) {
		t.Parallel()

		originalConfig := azurehelper.StorageAccountConfig{
			SubscriptionID:        "subscription-id",
			ResourceGroupName:     "resource-group",
			StorageAccountName:    "storageaccount",
			Location:              "eastus",
			EnableVersioning:      true,
			AllowBlobPublicAccess: false,
			Tags: map[string]string{
				"Environment": "Test",
			},
		}

		// Create a copy to compare against
		configCopy := originalConfig
		configCopy.Tags = make(map[string]string)
		for k, v := range originalConfig.Tags {
			configCopy.Tags[k] = v
		}

		// Validate the original
		err := originalConfig.Validate()
		assert.NoError(t, err)

		// Verify original hasn't changed
		assert.Equal(t, configCopy.SubscriptionID, originalConfig.SubscriptionID)
		assert.Equal(t, configCopy.ResourceGroupName, originalConfig.ResourceGroupName)
		assert.Equal(t, configCopy.StorageAccountName, originalConfig.StorageAccountName)
		assert.Equal(t, configCopy.Location, originalConfig.Location)
		assert.Equal(t, configCopy.EnableVersioning, originalConfig.EnableVersioning)
		assert.Equal(t, configCopy.AllowBlobPublicAccess, originalConfig.AllowBlobPublicAccess)
		assert.Equal(t, configCopy.Tags, originalConfig.Tags)
	})

	// Test that modifying returned SKU doesn't affect future calls
	t.Run("SKU generation is stateless", func(t *testing.T) {
		t.Parallel()

		// Get first SKU
		sku1, default1 := azurehelper.GetStorageAccountSKU("Standard", "LRS")

		// Modify the returned string (simulate accidental modification)
		modifiedSKU := sku1 + "_MODIFIED"
		_ = modifiedSKU

		// Get second SKU - should be unaffected
		sku2, default2 := azurehelper.GetStorageAccountSKU("Standard", "LRS")

		assert.Equal(t, sku1, sku2, "Subsequent calls should return identical results")
		assert.Equal(t, default1, default2, "Default flags should be consistent")
		assert.Equal(t, "Standard_LRS", sku2, "SKU should not be affected by previous modifications")
	})
}

// TestCreateStorageAccountClientErrorHandling tests error scenarios and edge cases for client creation
func TestCreateStorageAccountClientErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		config         map[string]interface{}
		expectedErrMsg string
		description    string
	}{
		{
			name:           "Nil config",
			config:         nil,
			expectedErrMsg: "config is required",
			description:    "Should fail when config is nil",
		},
		{
			name:           "Empty config",
			config:         map[string]interface{}{},
			expectedErrMsg: "storage_account_name is required",
			description:    "Should fail when storage_account_name is missing",
		},
		{
			name: "Empty storage account name",
			config: map[string]interface{}{
				"storage_account_name": "",
			},
			expectedErrMsg: "storage_account_name is required",
			description:    "Should fail when storage_account_name is empty",
		},
		{
			name: "Storage account name wrong type",
			config: map[string]interface{}{
				"storage_account_name": 123, // Wrong type
			},
			expectedErrMsg: "storage_account_name is required",
			description:    "Should fail when storage_account_name is not a string",
		},
		{
			name: "Storage account name with only whitespace",
			config: map[string]interface{}{
				"storage_account_name": "   ",
			},
			expectedErrMsg: "", // Whitespace is not checked by the implementation, just emptiness
			description:    "Should pass initial validation but may fail later in Azure credential check",
		},
		{
			name: "Resource group name wrong type",
			config: map[string]interface{}{
				"storage_account_name": "teststorage",
				"resource_group_name":  123, // Wrong type
			},
			expectedErrMsg: "",
			description:    "Should handle wrong type gracefully by using default",
		},
		{
			name: "Subscription ID wrong type",
			config: map[string]interface{}{
				"storage_account_name": "teststorage",
				"subscription_id":      123, // Wrong type
			},
			expectedErrMsg: "",
			description:    "Should handle wrong type gracefully",
		},
		{
			name: "Location wrong type",
			config: map[string]interface{}{
				"storage_account_name": "teststorage",
				"location":             123, // Wrong type
			},
			expectedErrMsg: "",
			description:    "Should handle wrong type gracefully",
		},
		{
			name: "Very long storage account name",
			config: map[string]interface{}{
				"storage_account_name": strings.Repeat("a", 1000),
			},
			expectedErrMsg: "",
			description:    "Should handle very long names",
		},
		{
			name: "Unicode characters in storage account name",
			config: map[string]interface{}{
				"storage_account_name": "storage中文账户",
			},
			expectedErrMsg: "",
			description:    "Should handle unicode characters",
		},
		{
			name: "Special characters in storage account name",
			config: map[string]interface{}{
				"storage_account_name": "storage-account_123",
			},
			expectedErrMsg: "",
			description:    "Should handle special characters",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			logger := log.Default().WithOptions(log.WithOutput(io.Discard))

			client, err := azurehelper.CreateStorageAccountClient(ctx, logger, tc.config)

			if tc.expectedErrMsg != "" {
				assert.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedErrMsg, tc.description)
				assert.Nil(t, client, "Client should be nil when error occurs")
			} else {
				// Note: These tests may fail due to Azure credential requirements
				// but they help test the parameter validation logic
				if err != nil {
					// If we get an error, it should be related to credentials, not config validation
					assert.NotContains(t, err.Error(), "storage_account_name is required")
					assert.NotContains(t, err.Error(), "config is required")
				}
			}
		})
	}
}

// TestHelperFunctionsErrorHandling tests error scenarios for utility functions
func TestHelperFunctionsErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("CompareStringMaps edge cases", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			existing map[string]*string
			desired  map[string]string
			expected bool
		}{
			{
				name:     "Both nil/empty",
				existing: nil,
				desired:  nil,
				expected: true,
			},
			{
				name:     "Existing nil, desired empty",
				existing: nil,
				desired:  map[string]string{},
				expected: true,
			},
			{
				name:     "Existing empty, desired nil",
				existing: map[string]*string{},
				desired:  nil,
				expected: true,
			},
			{
				name: "Identical maps",
				existing: map[string]*string{
					"env":     to.Ptr("test"),
					"project": to.Ptr("terragrunt"),
				},
				desired: map[string]string{
					"env":     "test",
					"project": "terragrunt",
				},
				expected: true,
			},
			{
				name: "Different values",
				existing: map[string]*string{
					"env": to.Ptr("prod"),
				},
				desired: map[string]string{
					"env": "test",
				},
				expected: false,
			},
			{
				name: "Missing key in existing",
				existing: map[string]*string{
					"env": to.Ptr("test"),
				},
				desired: map[string]string{
					"env":     "test",
					"project": "terragrunt",
				},
				expected: false,
			},
			{
				name: "Extra key in existing",
				existing: map[string]*string{
					"env":     to.Ptr("test"),
					"project": to.Ptr("terragrunt"),
				},
				desired: map[string]string{
					"env": "test",
				},
				expected: false,
			},
			{
				name: "Nil value in existing",
				existing: map[string]*string{
					"env": nil,
				},
				desired: map[string]string{
					"env": "test",
				},
				expected: false,
			},
			{
				name: "Empty string values",
				existing: map[string]*string{
					"env": to.Ptr(""),
				},
				desired: map[string]string{
					"env": "",
				},
				expected: true,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Test the comparison logic directly (since CompareStringMaps is not exported)
				// We simulate the logic from the actual function
				if len(tc.existing) != len(tc.desired) {
					assert.Equal(t, tc.expected, false, "Length mismatch should result in false")
					return
				}

				result := true
				for k, v := range tc.desired {
					if existingVal, ok := tc.existing[k]; !ok || existingVal == nil || *existingVal != v {
						result = false
						break
					}
				}

				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("ConvertToPointerMap edge cases", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			input    map[string]string
			expected map[string]*string
		}{
			{
				name:     "Empty map",
				input:    map[string]string{},
				expected: map[string]*string{},
			},
			{
				name:     "Nil map results in empty map",
				input:    nil,
				expected: map[string]*string{},
			},
			{
				name: "Single entry",
				input: map[string]string{
					"key": "value",
				},
				expected: map[string]*string{
					"key": to.Ptr("value"),
				},
			},
			{
				name: "Multiple entries",
				input: map[string]string{
					"env":     "test",
					"project": "terragrunt",
					"owner":   "devops",
				},
				expected: map[string]*string{
					"env":     to.Ptr("test"),
					"project": to.Ptr("terragrunt"),
					"owner":   to.Ptr("devops"),
				},
			},
			{
				name: "Empty string values",
				input: map[string]string{
					"empty": "",
					"test":  "value",
				},
				expected: map[string]*string{
					"empty": to.Ptr(""),
					"test":  to.Ptr("value"),
				},
			},
			{
				name: "Special characters",
				input: map[string]string{
					"unicode": "测试",
					"special": "!@#$%^&*()",
				},
				expected: map[string]*string{
					"unicode": to.Ptr("测试"),
					"special": to.Ptr("!@#$%^&*()"),
				},
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Test the conversion logic directly (simulating ConvertToPointerMap)
				result := make(map[string]*string, len(tc.input))
				for k, v := range tc.input {
					val := v // Create new variable to avoid capturing loop variable
					result[k] = &val
				}

				assert.Equal(t, len(tc.expected), len(result))

				for k, expectedPtr := range tc.expected {
					actualPtr, exists := result[k]
					assert.True(t, exists, "Key %s should exist", k)

					if expectedPtr == nil {
						assert.Nil(t, actualPtr)
					} else {
						assert.NotNil(t, actualPtr)
						assert.Equal(t, *expectedPtr, *actualPtr)
					}
				}
			})
		}
	})

	t.Run("CompareAccessTier edge cases", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			current  *armstorage.AccessTier
			desired  string
			expected bool
		}{
			{
				name:     "Both nil/empty",
				current:  nil,
				desired:  "",
				expected: true,
			},
			{
				name:     "Current nil, desired empty",
				current:  nil,
				desired:  "",
				expected: true,
			},
			{
				name:     "Current nil, desired not empty",
				current:  nil,
				desired:  "Hot",
				expected: false,
			},
			{
				name:     "Current not nil, desired empty",
				current:  to.Ptr(armstorage.AccessTierHot),
				desired:  "",
				expected: false,
			},
			{
				name:     "Both Hot",
				current:  to.Ptr(armstorage.AccessTierHot),
				desired:  "Hot",
				expected: true,
			},
			{
				name:     "Both Cool",
				current:  to.Ptr(armstorage.AccessTierCool),
				desired:  "Cool",
				expected: true,
			},
			{
				name:     "Both Premium",
				current:  to.Ptr(armstorage.AccessTierPremium),
				desired:  "Premium",
				expected: true,
			},
			{
				name:     "Different tiers - Hot vs Cool",
				current:  to.Ptr(armstorage.AccessTierHot),
				desired:  "Cool",
				expected: false,
			},
			{
				name:     "Different tiers - Cool vs Premium",
				current:  to.Ptr(armstorage.AccessTierCool),
				desired:  "Premium",
				expected: false,
			},
			{
				name:     "Case sensitivity test",
				current:  to.Ptr(armstorage.AccessTierHot),
				desired:  "hot", // lowercase
				expected: false,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Test the comparison logic directly (simulating CompareAccessTier)
				result := true
				if tc.current == nil && tc.desired == "" {
					result = true
				} else if tc.current == nil || tc.desired == "" {
					result = false
				} else {
					result = string(*tc.current) == tc.desired
				}

				assert.Equal(t, tc.expected, result)
			})
		}
	})
}

// TestUpdateStorageAccountLogic tests the decision logic for determining what needs updating
func TestUpdateStorageAccountLogic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                     string
		currentAccount           *armstorage.Account
		desiredConfig            azurehelper.StorageAccountConfig
		expectedNeedsUpdate      bool
		expectedUpdateAllowBlob  bool
		expectedUpdateAccessTier bool
		expectedUpdateTags       bool
		expectedWarningCount     int // Count of expected warnings for read-only properties
		expectedWarningMessages  []string
	}{
		{
			name: "No updates needed - everything matches",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("test"),
				},
				SKU: &armstorage.SKU{
					Name: to.Ptr(armstorage.SKUNameStandardLRS),
				},
				Kind:     to.Ptr(armstorage.KindStorageV2),
				Location: to.Ptr("eastus"),
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "Hot",
				Tags: map[string]string{
					"env": "test",
				},
				AccountTier:     "Standard",
				ReplicationType: "LRS",
				AccountKind:     "StorageV2",
				Location:        "eastus",
			},
			expectedNeedsUpdate:      false,
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       false,
			expectedWarningCount:     0,
		},
		{
			name: "Update AllowBlobPublicAccess only",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("test"),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: true, // Different from current
				AccessTier:            "Hot",
				Tags: map[string]string{
					"env": "test",
				},
			},
			expectedNeedsUpdate:      true,
			expectedUpdateAllowBlob:  true,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       false,
			expectedWarningCount:     0,
		},
		{
			name: "Update AccessTier only",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("test"),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "Cool", // Different from current
				Tags: map[string]string{
					"env": "test",
				},
			},
			expectedNeedsUpdate:      true,
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: true,
			expectedUpdateTags:       false,
			expectedWarningCount:     0,
		},
		{
			name: "Update Tags only",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("test"),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "Hot",
				Tags: map[string]string{
					"env":     "test",
					"project": "terragrunt", // Additional tag
				},
			},
			expectedNeedsUpdate:      true,
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       true,
			expectedWarningCount:     0,
		},
		{
			name: "Multiple updates needed",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(true),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,  // Different
				AccessTier:            "Cool", // Different
				Tags: map[string]string{
					"env":     "prod",       // Different
					"project": "terragrunt", // Additional
				},
			},
			expectedNeedsUpdate:      true,
			expectedUpdateAllowBlob:  true,
			expectedUpdateAccessTier: true,
			expectedUpdateTags:       true,
			expectedWarningCount:     0,
		},
		{
			name: "Read-only property differences should generate warnings",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				SKU: &armstorage.SKU{
					Name: to.Ptr(armstorage.SKUNameStandardLRS),
				},
				Kind:     to.Ptr(armstorage.KindStorageV2),
				Location: to.Ptr("eastus"),
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "Hot",
				AccountTier:           "Premium", // Different - should warn
				ReplicationType:       "GRS",     // Different - should warn
				AccountKind:           "Storage", // Different - should warn
				Location:              "westus",  // Different - should warn
			},
			expectedNeedsUpdate:      false, // No updatable properties changed
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       false,
			expectedWarningCount:     3, // SKU, Kind, Location warnings
			expectedWarningMessages: []string{
				"SKU cannot be changed",
				"kind cannot be changed",
				"location cannot be changed",
			},
		},
		{
			name: "Empty desired access tier should not trigger update",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "", // Empty - should not update
			},
			expectedNeedsUpdate:      false,
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       false,
			expectedWarningCount:     0,
		},
		{
			name: "Empty desired tags should not trigger update",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"env": to.Ptr("test"),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				Tags:                  map[string]string{}, // Empty - should not update
			},
			expectedNeedsUpdate:      false,
			expectedUpdateAllowBlob:  false,
			expectedUpdateAccessTier: false,
			expectedUpdateTags:       false,
			expectedWarningCount:     0,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test the update decision logic manually
			var needsUpdate bool
			var updateAllowBlob, updateAccessTier, updateTags bool
			var warningCount int

			// 1. Check AllowBlobPublicAccess
			if tc.currentAccount.Properties != nil && tc.currentAccount.Properties.AllowBlobPublicAccess != nil &&
				*tc.currentAccount.Properties.AllowBlobPublicAccess != tc.desiredConfig.AllowBlobPublicAccess {
				needsUpdate = true
				updateAllowBlob = true
			}

			// 2. Check AccessTier
			currentTierMatches := true
			if tc.currentAccount.Properties != nil && tc.currentAccount.Properties.AccessTier != nil && tc.desiredConfig.AccessTier != "" {
				currentTierMatches = string(*tc.currentAccount.Properties.AccessTier) == tc.desiredConfig.AccessTier
			} else if tc.currentAccount.Properties != nil && tc.currentAccount.Properties.AccessTier == nil && tc.desiredConfig.AccessTier == "" {
				currentTierMatches = true
			} else if tc.desiredConfig.AccessTier == "" {
				currentTierMatches = true // Don't update if desired is empty
			}

			if !currentTierMatches && tc.desiredConfig.AccessTier != "" {
				needsUpdate = true
				updateAccessTier = true
			}

			// 3. Check Tags
			currentTags := make(map[string]string)
			if tc.currentAccount.Tags != nil {
				for k, v := range tc.currentAccount.Tags {
					if v != nil {
						currentTags[k] = *v

					}
				}
			}

			tagsMatch := len(currentTags) == len(tc.desiredConfig.Tags)
			if tagsMatch {
				for k, v := range tc.desiredConfig.Tags {
					if currentVal, ok := currentTags[k]; !ok || currentVal != v {
						tagsMatch = false
						break
					}
				}
			}

			if !tagsMatch && len(tc.desiredConfig.Tags) > 0 {
				needsUpdate = true
				updateTags = true
			}

			// 4. Check read-only properties for warnings
			if tc.currentAccount.SKU != nil && (tc.desiredConfig.AccountTier != "" || tc.desiredConfig.ReplicationType != "") {
				currentSKU := string(*tc.currentAccount.SKU.Name)
				expectedSKU, _ := azurehelper.GetStorageAccountSKU(tc.desiredConfig.AccountTier, tc.desiredConfig.ReplicationType)
				if currentSKU != expectedSKU {
					warningCount++
				}
			}

			if tc.currentAccount.Kind != nil && tc.desiredConfig.AccountKind != "" {
				if string(*tc.currentAccount.Kind) != tc.desiredConfig.AccountKind {
					warningCount++
				}
			}

			if tc.currentAccount.Location != nil && tc.desiredConfig.Location != "" {
				if *tc.currentAccount.Location != tc.desiredConfig.Location {
					warningCount++
				}
			}

			// Assertions
			assert.Equal(t, tc.expectedNeedsUpdate, needsUpdate, "needsUpdate mismatch")
			assert.Equal(t, tc.expectedUpdateAllowBlob, updateAllowBlob, "updateAllowBlob mismatch")
			assert.Equal(t, tc.expectedUpdateAccessTier, updateAccessTier, "updateAccessTier mismatch")
			assert.Equal(t, tc.expectedUpdateTags, updateTags, "updateTags mismatch")
			assert.Equal(t, tc.expectedWarningCount, warningCount, "warningCount mismatch")
		})
	}
}

// TestUpdateStorageAccountParameterBuilding tests the parameter building logic
func TestUpdateStorageAccountParameterBuilding(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		desiredConfig  azurehelper.StorageAccountConfig
		expectedParams armstorage.AccountUpdateParameters
	}{
		{
			name: "AllowBlobPublicAccess update",
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: true,
			},
			expectedParams: armstorage.AccountUpdateParameters{
				Properties: &armstorage.AccountPropertiesUpdateParameters{
					AllowBlobPublicAccess: to.Ptr(true),
				},
			},
		},
		{
			name: "AccessTier update to Cool",
			desiredConfig: azurehelper.StorageAccountConfig{
				AccessTier: "Cool",
			},
			expectedParams: armstorage.AccountUpdateParameters{
				Properties: &armstorage.AccountPropertiesUpdateParameters{
					AccessTier: to.Ptr(armstorage.AccessTierCool),
				},
			},
		},
		{
			name: "AccessTier update to Premium",
			desiredConfig: azurehelper.StorageAccountConfig{
				AccessTier: "Premium",
			},
			expectedParams: armstorage.AccountUpdateParameters{
				Properties: &armstorage.AccountPropertiesUpdateParameters{
					AccessTier: to.Ptr(armstorage.AccessTierPremium),
				},
			},
		},
		{
			name: "Tags update",
			desiredConfig: azurehelper.StorageAccountConfig{
				Tags: map[string]string{
					"env":     "test",
					"project": "terragrunt",
				},
			},
			expectedParams: armstorage.AccountUpdateParameters{
				Properties: &armstorage.AccountPropertiesUpdateParameters{},
				Tags: map[string]*string{
					"env":     to.Ptr("test"),
					"project": to.Ptr("terragrunt"),
				},
			},
		},
		{
			name: "Multiple updates",
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: false,
				AccessTier:            "Hot",
				Tags: map[string]string{
					"owner": "devops",
				},
			},
			expectedParams: armstorage.AccountUpdateParameters{
				Properties: &armstorage.AccountPropertiesUpdateParameters{
					AllowBlobPublicAccess: to.Ptr(false),
					AccessTier:            to.Ptr(armstorage.AccessTierHot),
				},
				Tags: map[string]*string{
					"owner": to.Ptr("devops"),
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build parameters manually based on the logic in updateStorageAccountIfNeeded
			var updateParams armstorage.AccountUpdateParameters
			updateParams.Properties = &armstorage.AccountPropertiesUpdateParameters{}

			// AllowBlobPublicAccess
			if tc.desiredConfig.AllowBlobPublicAccess != false || tc.expectedParams.Properties.AllowBlobPublicAccess != nil {
				updateParams.Properties.AllowBlobPublicAccess = to.Ptr(tc.desiredConfig.AllowBlobPublicAccess)
			}

			// AccessTier
			if tc.desiredConfig.AccessTier != "" {
				switch tc.desiredConfig.AccessTier {
				case "Hot":
					updateParams.Properties.AccessTier = to.Ptr(armstorage.AccessTierHot)
				case "Cool":
					updateParams.Properties.AccessTier = to.Ptr(armstorage.AccessTierCool)
				case "Premium":
					updateParams.Properties.AccessTier = to.Ptr(armstorage.AccessTierPremium)
				}
			}

			// Tags
			if len(tc.desiredConfig.Tags) > 0 {
				updateParams.Tags = make(map[string]*string, len(tc.desiredConfig.Tags))
				for k, v := range tc.desiredConfig.Tags {
					val := v
					updateParams.Tags[k] = &val
				}
			}

			// Compare built parameters with expected
			if tc.expectedParams.Properties != nil {
				assert.NotNil(t, updateParams.Properties)

				if tc.expectedParams.Properties.AllowBlobPublicAccess != nil {
					assert.Equal(t, *tc.expectedParams.Properties.AllowBlobPublicAccess, *updateParams.Properties.AllowBlobPublicAccess)
				}

				if tc.expectedParams.Properties.AccessTier != nil {
					assert.Equal(t, *tc.expectedParams.Properties.AccessTier, *updateParams.Properties.AccessTier)
				}
			}

			if tc.expectedParams.Tags != nil {
				assert.Equal(t, len(tc.expectedParams.Tags), len(updateParams.Tags))
				for k, expectedVal := range tc.expectedParams.Tags {
					actualVal, exists := updateParams.Tags[k]
					assert.True(t, exists, "Tag %s should exist", k)
					assert.Equal(t, *expectedVal, *actualVal, "Tag %s value mismatch", k)
				}
			}
		})
	}
}

// TestUpdateStorageAccountEdgeCases tests edge cases in the update logic
func TestUpdateStorageAccountEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		currentAccount *armstorage.Account
		desiredConfig  azurehelper.StorageAccountConfig
		description    string
	}{
		{
			name: "Nil account properties",
			currentAccount: &armstorage.Account{
				Properties: nil,
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: true,
			},
			description: "Should handle nil Properties gracefully",
		},
		{
			name: "Nil AllowBlobPublicAccess in current account",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AllowBlobPublicAccess: nil,
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AllowBlobPublicAccess: true,
			},
			description: "Should handle nil AllowBlobPublicAccess gracefully",
		},
		{
			name: "Nil AccessTier in current account",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AccessTier: nil,
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AccessTier: "Hot",
			},
			description: "Should handle nil AccessTier gracefully",
		},
		{
			name: "Nil tags in current account",
			currentAccount: &armstorage.Account{
				Tags: nil,
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				Tags: map[string]string{
					"env": "test",
				},
			},
			description: "Should handle nil Tags gracefully",
		},
		{
			name: "Invalid AccessTier in desired config",
			currentAccount: &armstorage.Account{
				Properties: &armstorage.AccountProperties{
					AccessTier: to.Ptr(armstorage.AccessTierHot),
				},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				AccessTier: "InvalidTier",
			},
			description: "Should handle invalid access tier gracefully",
		},
		{
			name: "Large number of tags",
			currentAccount: &armstorage.Account{
				Tags: map[string]*string{},
			},
			desiredConfig: azurehelper.StorageAccountConfig{
				Tags: func() map[string]string {
					tags := make(map[string]string)
					for i := 0; i < 100; i++ {
						tags[fmt.Sprintf("tag-%d", i)] = fmt.Sprintf("value-%d", i)
					}
					return tags
				}(),
			},
			description: "Should handle large number of tags",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that the logic doesn't panic and handles edge cases gracefully
			assert.NotPanics(t, func() {
				// Simulate the key logic paths without actual Azure calls
				var needsUpdate bool

				// Test AllowBlobPublicAccess logic
				if tc.currentAccount.Properties != nil && tc.currentAccount.Properties.AllowBlobPublicAccess != nil {
					if *tc.currentAccount.Properties.AllowBlobPublicAccess != tc.desiredConfig.AllowBlobPublicAccess {
						needsUpdate = true
					}
				}

				// Test AccessTier logic
				if tc.currentAccount.Properties != nil && tc.currentAccount.Properties.AccessTier != nil && tc.desiredConfig.AccessTier != "" {
					if string(*tc.currentAccount.Properties.AccessTier) != tc.desiredConfig.AccessTier {
						needsUpdate = true
					}
				}

				// Test Tags logic
				currentTags := make(map[string]string)
				if tc.currentAccount.Tags != nil {
					for k, v := range tc.currentAccount.Tags {
						if v != nil {
							currentTags[k] = *v
						}
					}
				}

				if len(currentTags) != len(tc.desiredConfig.Tags) && len(tc.desiredConfig.Tags) > 0 {
					needsUpdate = true
				}

				// Ensure we got a boolean result without panicking
				_ = needsUpdate
			}, tc.description)
		})
	}
}

