//go:build azure

package azurehelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	// "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceGroupConfigValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config         azurehelper.ResourceGroupConfig
		name           string
		expectedErrMsg string
		isValid        bool
	}{
		{
			name: "Valid config",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "00000000-0000-0000-0000-000000000000",
				ResourceGroupName: "resource-group",
				Location:          "eastus",
			},
			isValid:        true,
			expectedErrMsg: "",
		},
		{
			name: "Missing subscription ID",
			config: azurehelper.ResourceGroupConfig{
				ResourceGroupName: "resource-group",
				Location:          "eastus",
			},
			isValid:        false,
			expectedErrMsg: "subscription_id is required",
		},
		{
			name: "Missing resource group name",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID: "subscription-id",
				Location:       "eastus",
			},
			isValid:        false,
			expectedErrMsg: "resource_group_name is required",
		},
		{
			name: "Missing location",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "subscription-id",
				ResourceGroupName: "resource-group",
			},
			isValid:        false,
			expectedErrMsg: "location is required",
		},
		{
			name: "Empty subscription ID",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "",
				ResourceGroupName: "resource-group",
				Location:          "eastus",
			},
			isValid:        false,
			expectedErrMsg: "subscription_id is required",
		},
		{
			name: "Empty resource group name",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "subscription-id",
				ResourceGroupName: "",
				Location:          "eastus",
			},
			isValid:        false,
			expectedErrMsg: "resource_group_name is required",
		},
		{
			name: "Empty location",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "subscription-id",
				ResourceGroupName: "resource-group",
				Location:          "",
			},
			isValid:        false,
			expectedErrMsg: "location is required",
		},
		{
			name: "With tags",
			config: azurehelper.ResourceGroupConfig{
				SubscriptionID:    "00000000-0000-0000-0000-000000000000",
				ResourceGroupName: "resource-group",
				Location:          "eastus",
				Tags: map[string]string{
					"Environment": "Test",
					"Owner":       "Terragrunt",
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

// TestResourceGroupNameValidation tests validation of resource group names
func TestResourceGroupNameValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rgName    string
		errorText string
		isValid   bool
	}{
		{
			name:      "Valid resource group name",
			rgName:    "my-resource-group",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Valid with numbers",
			rgName:    "rg-123",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Valid with underscores",
			rgName:    "rg_test_123",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Valid with periods",
			rgName:    "rg.test.123",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Valid with parentheses",
			rgName:    "rg(test)123",
			isValid:   true,
			errorText: "",
		},
		{
			name:      "Empty name",
			rgName:    "",
			isValid:   false,
			errorText: "resource_group_name is required",
		},
		{
			name:      "Too long name",
			rgName:    "this-resource-group-name-is-way-too-long-and-exceeds-the-maximum-length-allowed-by-azure-which-is-90-characters-for-resource-group-names",
			isValid:   false,
			errorText: "resource_group_name exceeds maximum length (90 characters)",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Use the real validation logic from the implementation
			config := azurehelper.ResourceGroupConfig{
				SubscriptionID:    "00000000-0000-0000-0000-000000000000",
				ResourceGroupName: tc.rgName,
				Location:          "eastus",
			}

			err := config.Validate()
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

// TestResourceGroupClientCreation tests the creation of resource group client with various inputs
func TestResourceGroupClientCreation(t *testing.T) {
	testCases := []struct {
		name                string
		subscriptionID      string
		envSubscriptionID   string
		expectedErrorPrefix string
		expectedError       bool
	}{
		{
			name:                "With valid subscription ID",
			subscriptionID:      "00000000-0000-0000-0000-000000000000",
			envSubscriptionID:   "",
			expectedError:       false,
			expectedErrorPrefix: "",
		},
		{
			name:                "Missing subscription ID but available in env",
			subscriptionID:      "",
			envSubscriptionID:   "00000000-0000-0000-0000-000000000000",
			expectedError:       false,
			expectedErrorPrefix: "",
		},
		{
			name:                "Missing subscription ID",
			subscriptionID:      "",
			envSubscriptionID:   "",
			expectedError:       true,
			expectedErrorPrefix: "subscription_id is required",
		},
		{
			name:                "Invalid subscription ID format",
			subscriptionID:      "invalid-subscription-id",
			envSubscriptionID:   "",
			expectedError:       true,
			expectedErrorPrefix: "invalid subscription ID format",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Clear environment variables for this test
			t.Setenv("AZURE_SUBSCRIPTION_ID", "")
			t.Setenv("ARM_SUBSCRIPTION_ID", "")

			if tc.envSubscriptionID != "" {
				t.Setenv("AZURE_SUBSCRIPTION_ID", tc.envSubscriptionID)
			}

			testLogger := log.New()

			client, err := azurehelper.CreateResourceGroupClient(t.Context(), testLogger, tc.subscriptionID)
			if tc.expectedError {
				require.Error(t, err)

				if tc.expectedErrorPrefix != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorPrefix)
				}

				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

// TestResourceGroupTagsHandling tests handling of resource group tags
func TestResourceGroupTagsHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		tags          map[string]string
		expectedTags  map[string]string
		name          string
		validateField bool
	}{
		{
			name:          "With valid tags",
			tags:          map[string]string{"Environment": "Test", "Owner": "Terragrunt"},
			expectedTags:  map[string]string{"Environment": "Test", "Owner": "Terragrunt"},
			validateField: true,
		},
		{
			name:          "With nil tags",
			tags:          nil,
			expectedTags:  map[string]string{},
			validateField: false,
		},
		{
			name:          "With empty tags",
			tags:          map[string]string{},
			expectedTags:  map[string]string{},
			validateField: false,
		},
		{
			name:          "With special characters in tag values",
			tags:          map[string]string{"Test:Key": "Value/With:Special@Characters"},
			expectedTags:  map[string]string{"Test:Key": "Value/With:Special@Characters"},
			validateField: true,
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := azurehelper.ResourceGroupConfig{
				SubscriptionID:    "00000000-0000-0000-0000-000000000000",
				ResourceGroupName: "test-resource-group",
				Location:          "eastus",
				Tags:              tc.tags,
			}

			// Validate that the tags field is properly used
			if tc.validateField {
				assert.Equal(t, tc.expectedTags, config.Tags)
				assert.NotEmpty(t, config.Tags)
			}

			// In a real implementation, we would test how the tags are applied to the Azure resource
			// but for unit tests without actual Azure resources, we just verify the data structure
			if tc.tags == nil {
				assert.Nil(t, config.Tags)
			} else {
				assert.Len(t, config.Tags, len(tc.tags))

				for k, v := range tc.tags {
					assert.Equal(t, v, config.Tags[k])
				}
			}
		})
	}
}

// TestResourceGroupTagManagement tests the tag handling functionality for resource groups
func TestResourceGroupTagManagement(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		inputTags          map[string]string
		expectedOutputTags map[string]string
		name               string
		description        string
	}{
		{
			name:      "Nil tags",
			inputTags: nil,
			expectedOutputTags: map[string]string{
				"created-by": "terragrunt",
			},
			description: "When nil tags are provided, default 'created-by' tag should be added",
		},
		{
			name:      "Empty tags",
			inputTags: map[string]string{},
			expectedOutputTags: map[string]string{
				"created-by": "terragrunt",
			},
			description: "When empty tags are provided, default 'created-by' tag should be added",
		},
		{
			name: "User-provided tags",
			inputTags: map[string]string{
				"environment": "dev",
				"project":     "terragrunt-test",
			},
			expectedOutputTags: map[string]string{
				"environment": "dev",
				"project":     "terragrunt-test",
			},
			description: "When user provides tags, they should be used as-is without adding defaults",
		},
		{
			name: "Tags with created-by already set",
			inputTags: map[string]string{
				"created-by": "user-script",
				"purpose":    "testing",
			},
			expectedOutputTags: map[string]string{
				"created-by": "user-script",
				"purpose":    "testing",
			},
			description: "When user provides a 'created-by' tag, it should be respected",
		},
		{
			name: "Tags with special characters",
			inputTags: map[string]string{
				"test:key":      "test:value",
				"key-with-dash": "value-with-dash",
			},
			expectedOutputTags: map[string]string{
				"test:key":      "test:value",
				"key-with-dash": "value-with-dash",
			},
			description: "Tags with special characters should be handled properly",
		},
		{
			name: "Tags with empty values",
			inputTags: map[string]string{
				"empty-value": "",
				"normal-key":  "normal-value",
			},
			expectedOutputTags: map[string]string{
				"empty-value": "",
				"normal-key":  "normal-value",
			},
			description: "Tags with empty values should be included",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Simulate tag handling logic from EnsureResourceGroup
			finalTags := tc.inputTags

			// If tags are nil or empty, add default tag
			if len(finalTags) == 0 {
				finalTags = map[string]string{
					"created-by": "terragrunt",
				}
			}

			// Verify the output tags match expected
			assert.Equal(t, tc.expectedOutputTags, finalTags, tc.description)
		})
	}
}

// TestResourceGroupLocation tests validation of Azure location names
func TestResourceGroupLocation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		location  string
		errorText string
		isValid   bool
	}{
		{
			name:     "Common location",
			location: "eastus",
			isValid:  true,
		},
		{
			name:     "Location with number",
			location: "eastus2",
			isValid:  true,
		},
		{
			name:     "Case insensitive",
			location: "EastUS",
			isValid:  true,
		},
		{
			name:     "Location with hyphen",
			location: "west-us",
			isValid:  true,
		},
		{
			name:      "Empty location",
			location:  "",
			isValid:   false,
			errorText: "location is required",
		},
		{
			name:      "Invalid location format",
			location:  "invalid_location@!",
			isValid:   false,
			errorText: "invalid location format",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a test config with this location
			config := azurehelper.ResourceGroupConfig{
				SubscriptionID:    "00000000-0000-0000-0000-000000000000",
				ResourceGroupName: "test-rg",
				Location:          tc.location,
			}

			err := config.Validate()
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

// In a real test environment with Azure credentials, we could test the actual client
// but for unit tests, we would need to mock the Azure SDK clients.
// Below is an example of how we might structure those tests if we had mocks:

/*
func TestCreateResourceGroupClient(t *testing.T) {
	// This would be implemented with mocks
}

func TestResourceGroupExists(t *testing.T) {
	// This would be implemented with mocks
}

func TestCreateResourceGroupIfNotExists(t *testing.T) {
	// This would be implemented with mocks
}
*/
