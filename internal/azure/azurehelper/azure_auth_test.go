//go:build azure

package azurehelper_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

// clearAzureEnvVars clears all Azure-related environment variables for clean test state.
// Using t.Setenv("VAR", "") ensures the variable is restored after the test.
func clearAzureEnvVars(t *testing.T) {
	t.Helper()

	azureEnvVars := []string{
		"AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET",
		"AZURE_TENANT_ID",
		"AZURE_SUBSCRIPTION_ID",
		"AZURE_MANAGED_IDENTITY_CLIENT_ID",
		"AZURE_MANAGED_IDENTITY_CLIENT_SECRET",
		"AZURE_STORAGE_CONNECTION_STRING",
		"AZURE_CLIENT_CERTIFICATE_PASSWORD",
		"ARM_CLIENT_ID",
		"ARM_CLIENT_SECRET",
		"ARM_TENANT_ID",
		"ARM_SUBSCRIPTION_ID",
	}
	for _, envVar := range azureEnvVars {
		t.Setenv(envVar, "")
	}
}

func TestAzureEnvironmentVariables(t *testing.T) {
	// Clear all Azure-related env vars to ensure clean test state
	clearAzureEnvVars(t)

	t.Setenv("AZURE_CLIENT_ID", "test-client-id")
	t.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("AZURE_TENANT_ID", "test-tenant-id")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "test-subscription-id")

	assert.Equal(t, "test-client-id", os.Getenv("AZURE_CLIENT_ID"))
	assert.Equal(t, "test-client-secret", os.Getenv("AZURE_CLIENT_SECRET"))
	assert.Equal(t, "test-tenant-id", os.Getenv("AZURE_TENANT_ID"))
	assert.Equal(t, "test-subscription-id", os.Getenv("AZURE_SUBSCRIPTION_ID"))
	assert.Empty(t, os.Getenv("ARM_CLIENT_ID"))
	assert.Empty(t, os.Getenv("ARM_SUBSCRIPTION_ID"))

	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	t.Setenv("ARM_CLIENT_ID", "arm-client-id")
	t.Setenv("ARM_SUBSCRIPTION_ID", "arm-subscription-id")

	assert.Empty(t, os.Getenv("AZURE_CLIENT_ID"))
	assert.Empty(t, os.Getenv("AZURE_SUBSCRIPTION_ID"))
	assert.Equal(t, "arm-client-id", os.Getenv("ARM_CLIENT_ID"))
	assert.Equal(t, "arm-subscription-id", os.Getenv("ARM_SUBSCRIPTION_ID"))
}

func TestAzureCredentialPriority(t *testing.T) {
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

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AZURE_CLIENT_ID", tc.azureClientID)
			t.Setenv("AZURE_SUBSCRIPTION_ID", tc.azureSubscriptionID)
			t.Setenv("ARM_CLIENT_ID", tc.armClientID)
			t.Setenv("ARM_SUBSCRIPTION_ID", tc.armSubscriptionID)

			// Call production code to verify credential resolution
			actualClientID := azurehelper.ResolveClientID()
			actualSubscriptionID := azurehelper.ResolveSubscriptionID()

			assert.Equal(t, tc.expectedClientID, actualClientID)
			assert.Equal(t, tc.expectedSubscriptionID, actualSubscriptionID)
		})
	}
}

func TestGetAzureCredentialsPriority(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Clear all Azure env vars first to ensure clean test state
			clearAzureEnvVars(t)

			// Now set only the test-specified values
			if tc.azureSubscriptionID != "" {
				t.Setenv("AZURE_SUBSCRIPTION_ID", tc.azureSubscriptionID)
			}

			if tc.azureTenantID != "" {
				t.Setenv("AZURE_TENANT_ID", tc.azureTenantID)
			}

			if tc.azureClientID != "" {
				t.Setenv("AZURE_CLIENT_ID", tc.azureClientID)
			}

			if tc.azureClientSecret != "" {
				t.Setenv("AZURE_CLIENT_SECRET", tc.azureClientSecret)
			}

			if tc.armSubscriptionID != "" {
				t.Setenv("ARM_SUBSCRIPTION_ID", tc.armSubscriptionID)
			}

			if tc.armTenantID != "" {
				t.Setenv("ARM_TENANT_ID", tc.armTenantID)
			}

			if tc.armClientID != "" {
				t.Setenv("ARM_CLIENT_ID", tc.armClientID)
			}

			if tc.armClientSecret != "" {
				t.Setenv("ARM_CLIENT_SECRET", tc.armClientSecret)
			}

			_, subscriptionID, err := azurehelper.GetAzureCredentials(t.Context(), createMockLogger())
			if tc.shouldHaveError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedSubscriptionID, subscriptionID)
			}
		})
	}
}

func TestAzureCredentialEnvironmentVariables(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Clear all Azure env vars first to ensure clean test state
			clearAzureEnvVars(t)

			// Now set only the test-specified values
			for key, val := range tc.setupEnvVars {
				t.Setenv(key, val)
			}

			// Call production code to verify subscription ID resolution
			actualSubID := azurehelper.ResolveSubscriptionID()

			assert.Equal(t, tc.expectedSubscriptionID, actualSubID)
		})
	}
}

func TestAzureSafeConfiguration(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			var logBuffer strings.Builder

			// Use the production redaction helper instead of duplicating logic
			for k := range tc.envVars {
				value := os.Getenv(k)
				redactedValue := azurehelper.RedactSensitiveValue(k, value)
				fmt.Fprintf(&logBuffer, "%s=%s\n", k, redactedValue)
			}

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

func TestRedactSensitiveValue(t *testing.T) {
	t.Parallel()

	const redacted = "***REDACTED***"

	testCases := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "Redact client secret",
			key:      "AZURE_CLIENT_SECRET",
			value:    "super-secret-value",
			expected: redacted,
		},
		{
			name:     "Redact certificate password",
			key:      "AZURE_CLIENT_CERTIFICATE_PASSWORD",
			value:    "cert-password",
			expected: redacted,
		},
		{
			name:     "Redact key containing _KEY",
			key:      "AZURE_STORAGE_KEY",
			value:    "storage-key-value",
			expected: redacted,
		},
		{
			name:     "Preserve non-sensitive value",
			key:      "AZURE_CLIENT_ID",
			value:    "client-id-value",
			expected: "client-id-value",
		},
		{
			name:     "Partially redact connection string with AccountKey",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "AccountName=myaccount;AccountKey=secret-key-123;EndpointSuffix=core.windows.net",
			expected: "AccountName=myaccount;AccountKey=" + redacted + ";EndpointSuffix=core.windows.net",
		},
		{
			name:     "Partially redact connection string with SharedAccessKey",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "AccountName=myaccount;SharedAccessKey=sas-token-456;",
			expected: "AccountName=myaccount;SharedAccessKey=" + redacted + ";",
		},
		{
			name:     "Partially redact connection string with both keys",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "AccountName=myaccount;AccountKey=key1;SharedAccessKey=key2;BlobEndpoint=https://myaccount.blob.core.windows.net/",
			expected: "AccountName=myaccount;AccountKey=" + redacted + ";SharedAccessKey=" + redacted + ";BlobEndpoint=https://myaccount.blob.core.windows.net/",
		},
		{
			name:     "Preserve connection string without sensitive keys",
			key:      "SOME_CONNECTION_STRING",
			value:    "Server=myserver;Database=mydb;",
			expected: "Server=myserver;Database=mydb;",
		},
		{
			name:     "Case-insensitive: lowercase accountkey",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "accountname=myaccount;accountkey=secret-key-123;endpointsuffix=core.windows.net",
			expected: "accountname=myaccount;accountkey=" + redacted + ";endpointsuffix=core.windows.net",
		},
		{
			name:     "Case-insensitive: UPPERCASE ACCOUNTKEY",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "ACCOUNTNAME=myaccount;ACCOUNTKEY=secret-key-123;ENDPOINTSUFFIX=core.windows.net",
			expected: "ACCOUNTNAME=myaccount;ACCOUNTKEY=" + redacted + ";ENDPOINTSUFFIX=core.windows.net",
		},
		{
			name:     "Case-insensitive: Mixed case SharedAccessKey",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "AccountName=myaccount;sharedaccesskey=sas-token-456;",
			expected: "AccountName=myaccount;sharedaccesskey=" + redacted + ";",
		},
		{
			name:     "Case-insensitive: Mixed case both keys",
			key:      "AZURE_STORAGE_CONNECTION_STRING",
			value:    "AccountName=myaccount;AccountKey=key1;SharedAccessKey=key2;",
			expected: "AccountName=myaccount;AccountKey=" + redacted + ";SharedAccessKey=" + redacted + ";",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := azurehelper.RedactSensitiveValue(tc.key, tc.value)
			assert.Equal(t, tc.expected, actual, "Redaction result should match expected value")
		})
	}
}
