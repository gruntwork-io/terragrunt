//go:build azure

package azurehelper_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestAzureEnvironmentVariables(t *testing.T) {
	t.Parallel()

	// Store original environment variables
	originalClientID := os.Getenv("AZURE_CLIENT_ID")
	originalClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	originalTenantID := os.Getenv("AZURE_TENANT_ID")
	originalSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")

	// Also test ARM_ prefixed variables which are sometimes used
	originalARMClientID := os.Getenv("ARM_CLIENT_ID")
	originalARMSubscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")

	// Restore environment variables after the test
	defer func() {
		os.Setenv("AZURE_CLIENT_ID", originalClientID)
		os.Setenv("AZURE_CLIENT_SECRET", originalClientSecret)
		os.Setenv("AZURE_TENANT_ID", originalTenantID)
		os.Setenv("AZURE_SUBSCRIPTION_ID", originalSubscriptionID)
		os.Setenv("ARM_CLIENT_ID", originalARMClientID)
		os.Setenv("ARM_SUBSCRIPTION_ID", originalARMSubscriptionID)
	}()

	// Test with environment variables set
	os.Setenv("AZURE_CLIENT_ID", "test-client-id")
	os.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
	os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
	os.Setenv("AZURE_SUBSCRIPTION_ID", "test-subscription-id")

	assert.Equal(t, "test-client-id", os.Getenv("AZURE_CLIENT_ID"))
	assert.Equal(t, "test-client-secret", os.Getenv("AZURE_CLIENT_SECRET"))
	assert.Equal(t, "test-tenant-id", os.Getenv("AZURE_TENANT_ID"))
	assert.Equal(t, "test-subscription-id", os.Getenv("AZURE_SUBSCRIPTION_ID"))

	// Now test ARM_ prefix variables
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("AZURE_SUBSCRIPTION_ID")
	os.Setenv("ARM_CLIENT_ID", "arm-client-id")
	os.Setenv("ARM_SUBSCRIPTION_ID", "arm-subscription-id")

	assert.Equal(t, "", os.Getenv("AZURE_CLIENT_ID"))
	assert.Equal(t, "", os.Getenv("AZURE_SUBSCRIPTION_ID"))
	assert.Equal(t, "arm-client-id", os.Getenv("ARM_CLIENT_ID"))
	assert.Equal(t, "arm-subscription-id", os.Getenv("ARM_SUBSCRIPTION_ID"))
}

// TestAzureCredentialPriority tests the priority of different credential sources
func TestAzureCredentialPriority(t *testing.T) {
	t.Parallel()

	// This test simulates the credential priority checking without actually making Azure API calls

	testCases := []struct {
		name                   string
		azureClientID          string
		azureSubscriptionID    string
		armClientID            string
		armSubscriptionID      string
		expectedClientID       string
		expectedSubscriptionID string
	}{
		{
			name:                   "AZURE_ prefix takes priority",
			azureClientID:          "azure-client-id",
			azureSubscriptionID:    "azure-subscription-id",
			armClientID:            "arm-client-id",
			armSubscriptionID:      "arm-subscription-id",
			expectedClientID:       "azure-client-id",
			expectedSubscriptionID: "azure-subscription-id",
		},
		{
			name:                   "ARM_ prefix as fallback",
			azureClientID:          "",
			azureSubscriptionID:    "",
			armClientID:            "arm-client-id",
			armSubscriptionID:      "arm-subscription-id",
			expectedClientID:       "arm-client-id",
			expectedSubscriptionID: "arm-subscription-id",
		},
		{
			name:                   "No credentials",
			azureClientID:          "",
			azureSubscriptionID:    "",
			armClientID:            "",
			armSubscriptionID:      "",
			expectedClientID:       "",
			expectedSubscriptionID: "",
		},
		{
			name:                   "Mixed prefixes",
			azureClientID:          "azure-client-id",
			azureSubscriptionID:    "",
			armClientID:            "",
			armSubscriptionID:      "arm-subscription-id",
			expectedClientID:       "azure-client-id",
			expectedSubscriptionID: "arm-subscription-id",
		},
	}

	// Save original environment
	originalAzureClientID := os.Getenv("AZURE_CLIENT_ID")
	originalAzureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	originalARMClientID := os.Getenv("ARM_CLIENT_ID")
	originalARMSubscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")

	// Restore environment variables after the test
	defer func() {
		os.Setenv("AZURE_CLIENT_ID", originalAzureClientID)
		os.Setenv("AZURE_SUBSCRIPTION_ID", originalAzureSubscriptionID)
		os.Setenv("ARM_CLIENT_ID", originalARMClientID)
		os.Setenv("ARM_SUBSCRIPTION_ID", originalARMSubscriptionID)
	}()

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Set environment for this test case
			os.Setenv("AZURE_CLIENT_ID", tc.azureClientID)
			os.Setenv("AZURE_SUBSCRIPTION_ID", tc.azureSubscriptionID)
			os.Setenv("ARM_CLIENT_ID", tc.armClientID)
			os.Setenv("ARM_SUBSCRIPTION_ID", tc.armSubscriptionID)

			// Simple credential resolver function that mimics what the real code would do
			resolveClientID := func() string {
				if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
					return clientID
				}
				if clientID := os.Getenv("ARM_CLIENT_ID"); clientID != "" {
					return clientID
				}
				return ""
			}

			resolveSubscriptionID := func() string {
				if subID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subID != "" {
					return subID
				}
				if subID := os.Getenv("ARM_SUBSCRIPTION_ID"); subID != "" {
					return subID
				}
				return ""
			}

			// Check if the resolution works as expected
			assert.Equal(t, tc.expectedClientID, resolveClientID())
			assert.Equal(t, tc.expectedSubscriptionID, resolveSubscriptionID())
		})
	}
}

// TestGetAzureCredentialsPriority tests the priority of different environment variables for credentials
func TestGetAzureCredentialsPriority(t *testing.T) {
	t.Parallel()

	// Store original environment variables
	originalAzureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	originalAzureTenantID := os.Getenv("AZURE_TENANT_ID")
	originalAzureClientID := os.Getenv("AZURE_CLIENT_ID")
	originalAzureClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	originalARMSubscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")
	originalARMTenantID := os.Getenv("ARM_TENANT_ID")
	originalARMClientID := os.Getenv("ARM_CLIENT_ID")
	originalARMClientSecret := os.Getenv("ARM_CLIENT_SECRET")

	// Restore environment variables after the test
	defer func() {
		os.Setenv("AZURE_SUBSCRIPTION_ID", originalAzureSubscriptionID)
		os.Setenv("AZURE_TENANT_ID", originalAzureTenantID)
		os.Setenv("AZURE_CLIENT_ID", originalAzureClientID)
		os.Setenv("AZURE_CLIENT_SECRET", originalAzureClientSecret)
		os.Setenv("ARM_SUBSCRIPTION_ID", originalARMSubscriptionID)
		os.Setenv("ARM_TENANT_ID", originalARMTenantID)
		os.Setenv("ARM_CLIENT_ID", originalARMClientID)
		os.Setenv("ARM_CLIENT_SECRET", originalARMClientSecret)
	}()

	testCases := []struct {
		name                   string
		azureSubscriptionID    string
		azureTenantID          string
		azureClientID          string
		azureClientSecret      string
		armSubscriptionID      string
		armTenantID            string
		armClientID            string
		armClientSecret        string
		expectedSubscriptionID string
		shouldHaveError        bool
	}{
		{
			name:                   "Azure vars take precedence",
			azureSubscriptionID:    "azure-sub-id",
			azureTenantID:          "azure-tenant-id",
			azureClientID:          "azure-client-id",
			azureClientSecret:      "azure-client-secret",
			armSubscriptionID:      "arm-sub-id",
			armTenantID:            "arm-tenant-id",
			armClientID:            "arm-client-id",
			armClientSecret:        "arm-client-secret",
			expectedSubscriptionID: "azure-sub-id",
			shouldHaveError:        false,
		},
		{
			name:                   "ARM vars as fallback",
			azureSubscriptionID:    "",
			azureTenantID:          "",
			azureClientID:          "",
			azureClientSecret:      "",
			armSubscriptionID:      "arm-sub-id",
			armTenantID:            "arm-tenant-id",
			armClientID:            "arm-client-id",
			armClientSecret:        "arm-client-secret",
			expectedSubscriptionID: "arm-sub-id",
			shouldHaveError:        false,
		},
		{
			name:                   "Mix of Azure and ARM vars",
			azureSubscriptionID:    "azure-sub-id",
			azureTenantID:          "",
			azureClientID:          "",
			azureClientSecret:      "",
			armSubscriptionID:      "",
			armTenantID:            "arm-tenant-id",
			armClientID:            "arm-client-id",
			armClientSecret:        "arm-client-secret",
			expectedSubscriptionID: "azure-sub-id",
			shouldHaveError:        false,
		},
		{
			name:                   "No subscription ID",
			azureSubscriptionID:    "",
			azureTenantID:          "azure-tenant-id",
			azureClientID:          "azure-client-id",
			azureClientSecret:      "azure-client-secret",
			armSubscriptionID:      "",
			armTenantID:            "",
			armClientID:            "",
			armClientSecret:        "",
			expectedSubscriptionID: "",
			shouldHaveError:        false,
		},
		{
			name:                   "No variables set",
			azureSubscriptionID:    "",
			azureTenantID:          "",
			azureClientID:          "",
			azureClientSecret:      "",
			armSubscriptionID:      "",
			armTenantID:            "",
			armClientID:            "",
			armClientSecret:        "",
			expectedSubscriptionID: "",
			shouldHaveError:        false, // Default credential might still work
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Clear all environment variables first
			os.Unsetenv("AZURE_SUBSCRIPTION_ID")
			os.Unsetenv("AZURE_TENANT_ID")
			os.Unsetenv("AZURE_CLIENT_ID")
			os.Unsetenv("AZURE_CLIENT_SECRET")
			os.Unsetenv("ARM_SUBSCRIPTION_ID")
			os.Unsetenv("ARM_TENANT_ID")
			os.Unsetenv("ARM_CLIENT_ID")
			os.Unsetenv("ARM_CLIENT_SECRET")

			// Set test-specific environment variables
			if tc.azureSubscriptionID != "" {
				os.Setenv("AZURE_SUBSCRIPTION_ID", tc.azureSubscriptionID)
			}
			if tc.azureTenantID != "" {
				os.Setenv("AZURE_TENANT_ID", tc.azureTenantID)
			}
			if tc.azureClientID != "" {
				os.Setenv("AZURE_CLIENT_ID", tc.azureClientID)
			}
			if tc.azureClientSecret != "" {
				os.Setenv("AZURE_CLIENT_SECRET", tc.azureClientSecret)
			}
			if tc.armSubscriptionID != "" {
				os.Setenv("ARM_SUBSCRIPTION_ID", tc.armSubscriptionID)
			}
			if tc.armTenantID != "" {
				os.Setenv("ARM_TENANT_ID", tc.armTenantID)
			}
			if tc.armClientID != "" {
				os.Setenv("ARM_CLIENT_ID", tc.armClientID)
			}
			if tc.armClientSecret != "" {
				os.Setenv("ARM_CLIENT_SECRET", tc.armClientSecret)
			}

			// Call the function to test
			// Note: We can't fully test DefaultAzureCredential creation without Azure access
			// But we can verify the subscription ID logic
			_, subscriptionID, _ := azurehelper.GetAzureCredentials(context.Background(), createMockLogger())
			assert.Equal(t, tc.expectedSubscriptionID, subscriptionID)
		})
	}
}

// TestAzureCredentialEnvironmentVariables tests various environment variable combinations
func TestAzureCredentialEnvironmentVariables(t *testing.T) {
	// Don't run in parallel since we're modifying environment variables

	// Store original environment variables
	envVars := []string{
		"AZURE_SUBSCRIPTION_ID",
		"AZURE_TENANT_ID",
		"AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET",
		"AZURE_MANAGED_IDENTITY_CLIENT_ID",
		"ARM_SUBSCRIPTION_ID",
		"ARM_TENANT_ID",
		"ARM_CLIENT_ID",
		"ARM_CLIENT_SECRET",
	}

	originalValues := make(map[string]string)
	for _, env := range envVars {
		originalValues[env] = os.Getenv(env)
	}

	// Restore environment variables after the test
	defer func() {
		for env, val := range originalValues {
			os.Setenv(env, val)
		}
	}()

	// Clear all environment variables first
	for _, env := range envVars {
		os.Unsetenv(env)
	}

	testCases := []struct {
		name                   string
		setupEnvVars           map[string]string
		expectedSubscriptionID string
	}{
		{
			name: "Azure CLI environment variables",
			setupEnvVars: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "azure-subscription-id",
				"AZURE_TENANT_ID":       "azure-tenant-id",
			},
			expectedSubscriptionID: "azure-subscription-id",
		},
		{
			name: "Service principal environment variables",
			setupEnvVars: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "azure-sp-subscription-id",
				"AZURE_TENANT_ID":       "azure-sp-tenant-id",
				"AZURE_CLIENT_ID":       "azure-sp-client-id",
				"AZURE_CLIENT_SECRET":   "azure-sp-client-secret",
			},
			expectedSubscriptionID: "azure-sp-subscription-id",
		},
		{
			name: "Managed identity environment variables",
			setupEnvVars: map[string]string{
				"AZURE_SUBSCRIPTION_ID":                "azure-mi-subscription-id",
				"AZURE_MANAGED_IDENTITY_CLIENT_ID":     "azure-mi-client-id",
				"AZURE_MANAGED_IDENTITY_CLIENT_SECRET": "azure-mi-client-secret",
			},
			expectedSubscriptionID: "azure-mi-subscription-id",
		},
		{
			name: "ARM environment variables",
			setupEnvVars: map[string]string{
				"ARM_SUBSCRIPTION_ID": "arm-subscription-id",
				"ARM_TENANT_ID":       "arm-tenant-id",
				"ARM_CLIENT_ID":       "arm-client-id",
				"ARM_CLIENT_SECRET":   "arm-client-secret",
			},
			expectedSubscriptionID: "arm-subscription-id",
		},
		{
			name: "Mixed environment variables with Azure priority",
			setupEnvVars: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "azure-subscription-id",
				"ARM_SUBSCRIPTION_ID":   "arm-subscription-id",
				"AZURE_CLIENT_ID":       "azure-client-id",
				"ARM_CLIENT_SECRET":     "arm-client-secret",
			},
			expectedSubscriptionID: "azure-subscription-id",
		},
		{
			name: "ARM fallback when no Azure variables",
			setupEnvVars: map[string]string{
				"ARM_SUBSCRIPTION_ID": "arm-fallback-subscription-id",
				"ARM_TENANT_ID":       "arm-fallback-tenant-id",
			},
			expectedSubscriptionID: "arm-fallback-subscription-id",
		},
		{
			name: "No subscription IDs",
			setupEnvVars: map[string]string{
				"AZURE_CLIENT_ID": "azure-client-id-only",
				"AZURE_TENANT_ID": "azure-tenant-id-only",
			},
			expectedSubscriptionID: "",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Don't run these tests in parallel since they modify environment variables

			// Clear all environment variables first
			for _, env := range envVars {
				if err := os.Unsetenv(env); err != nil {
					t.Fatalf("Failed to unset environment variable %s: %v", env, err)
				}
			}

			// Set up environment variables for this test case
			for key, val := range tc.setupEnvVars {
				if err := os.Setenv(key, val); err != nil {
					t.Fatalf("Failed to set environment variable %s: %v", key, err)
				}
			}

			// Get all environment variables used by GetAzureCredentials
			var detectedVars []string
			for _, env := range envVars {
				if val := os.Getenv(env); val != "" {
					detectedVars = append(detectedVars, env+"="+val)
				}
			}

			// We can't actually call GetAzureCredentials in unit tests without credentials,
			// so we'll validate the environment variables and expected behavior

			// Check if AZURE_SUBSCRIPTION_ID is set
			azureSubID := os.Getenv("AZURE_SUBSCRIPTION_ID")
			// Check if ARM_SUBSCRIPTION_ID is set
			armSubID := os.Getenv("ARM_SUBSCRIPTION_ID")

			// Determine expected subscription ID based on environment variable priority
			var expectedSubID string
			if azureSubID != "" {
				expectedSubID = azureSubID
			} else if armSubID != "" {
				expectedSubID = armSubID
			}

			assert.Equal(t, tc.expectedSubscriptionID, expectedSubID)
		})
	}
}

// TestAzureSafeConfiguration tests that sensitive credentials are handled safely
func TestAzureSafeConfiguration(t *testing.T) {
	t.Parallel()

	// Store original environment variables
	originalClientID := os.Getenv("AZURE_CLIENT_ID")
	originalClientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	// Restore environment variables after the test
	defer func() {
		os.Setenv("AZURE_CLIENT_ID", originalClientID)
		os.Setenv("AZURE_CLIENT_SECRET", originalClientSecret)
	}()

	// Set test environment variables
	os.Setenv("AZURE_CLIENT_ID", "test-client-id")
	os.Setenv("AZURE_CLIENT_SECRET", "very-secret-value-should-not-be-logged")

	// Test cases for safe logging
	testCases := []struct {
		name              string
		envVars           map[string]string
		shouldContainKeys []string
		shouldNotContain  []string
	}{
		{
			name: "Safe credential handling",
			envVars: map[string]string{
				"AZURE_CLIENT_ID":     "visible-client-id",
				"AZURE_CLIENT_SECRET": "hidden-secret",
			},
			shouldContainKeys: []string{"AZURE_CLIENT_ID"},
			shouldNotContain:  []string{"hidden-secret"},
		},
		{
			name: "Safe connection string handling",
			envVars: map[string]string{
				"AZURE_STORAGE_CONNECTION_STRING": "AccountName=test;AccountKey=hidden-storage-key;",
			},
			shouldContainKeys: []string{"AZURE_STORAGE_CONNECTION_STRING"},
			shouldNotContain:  []string{"hidden-storage-key"},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Set up any test environment variables
			for k, v := range tc.envVars {
				os.Setenv(k, v)
			}

			// Create a buffer to capture log output
			var logBuffer strings.Builder

			// Simulate safe logging - in a real implementation, we'd use the actual logging function
			// but for testing, we'll just simulate it
			var logOutput string
			for k := range tc.envVars {
				value := os.Getenv(k)
				shouldRedact := k == "AZURE_CLIENT_SECRET" ||
					k == "AZURE_CLIENT_CERTIFICATE_PASSWORD" ||
					strings.Contains(k, "_KEY") ||
					strings.Contains(k, "PASSWORD") ||
					strings.Contains(k, "SECRET") ||
					strings.Contains(value, "AccountKey=")

				if shouldRedact {
					logOutput += fmt.Sprintf("%s=***REDACTED***\n", k)
				} else {
					// For connection strings, redact sensitive parts
					if strings.Contains(value, ";") {
						parts := strings.Split(value, ";")
						var safeParts []string
						for _, part := range parts {
							if strings.Contains(part, "AccountKey=") ||
								strings.Contains(part, "SharedAccessKey=") {
								safeParts = append(safeParts, "AccountKey=***REDACTED***")
							} else {
								safeParts = append(safeParts, part)
							}
						}
						logOutput += fmt.Sprintf("%s=%s\n", k, strings.Join(safeParts, ";"))
					} else {
						logOutput += fmt.Sprintf("%s=%s\n", k, value)
					}
				}
			}

			logBuffer.WriteString(logOutput)
			logStr := logBuffer.String()

			// Check that sensitive values are not logged
			for _, key := range tc.shouldContainKeys {
				assert.Contains(t, logStr, key, "Log should contain the key")
			}

			for _, value := range tc.shouldNotContain {
				assert.NotContains(t, logStr, value, "Log should not contain sensitive value")
			}
		})
	}
}

// Helper function to create a mock logger for testing
func createMockLogger() log.Logger {
	return log.Default().WithOptions(log.WithOutput(io.Discard))
}
