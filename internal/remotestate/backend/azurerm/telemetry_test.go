package azurerm_test

import (
	"errors"
	"testing"
	"time"

	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		error         error
		expectedClass azurerm.ErrorClassification
	}{
		{
			name:          "nil error returns empty classification",
			error:         nil,
			expectedClass: "",
		},
		{
			name:          "authentication error with 401",
			error:         errors.New("HTTP 401: unauthorized access"),
			expectedClass: azurerm.ErrorClassAuthentication,
		},
		{
			name:          "authentication error with forbidden",
			error:         errors.New("forbidden: invalid credentials"),
			expectedClass: azurerm.ErrorClassAuthentication,
		},
		{
			name:          "authentication error with token",
			error:         errors.New("authentication failed: invalid token"),
			expectedClass: azurerm.ErrorClassAuthentication,
		},
		{
			name:          "missing subscription ID configuration",
			error:         errors.New("missing subscription_id configuration"),
			expectedClass: azurerm.ErrorClassConfiguration,
		},
		{
			name:          "missing location configuration",
			error:         errors.New("missing location parameter"),
			expectedClass: azurerm.ErrorClassConfiguration,
		},
		{
			name:          "missing resource group configuration",
			error:         errors.New("missing resource group name"),
			expectedClass: azurerm.ErrorClassConfiguration,
		},
		{
			name:          "storage account error",
			error:         errors.New("storage account creation failed"),
			expectedClass: azurerm.ErrorClassStorage,
		},
		{
			name:          "storage account not found",
			error:         errors.New("storage account 'test' does not exist"),
			expectedClass: azurerm.ErrorClassResourceNotFound,
		},
		{
			name:          "container error",
			error:         errors.New("container creation failed"),
			expectedClass: azurerm.ErrorClassContainer,
		},
		{
			name:          "container not found",
			error:         errors.New("container 'testcontainer' not found"),
			expectedClass: azurerm.ErrorClassResourceNotFound,
		},
		{
			name:          "container validation error",
			error:         errors.New("container name validation failed: must be lowercase"),
			expectedClass: azurerm.ErrorClassValidation,
		},
		{
			name:          "invalid parameter validation",
			error:         errors.New("parameter must be between 3 and 63 characters"),
			expectedClass: azurerm.ErrorClassValidation,
		},
		{
			name:          "network timeout",
			error:         errors.New("connection timeout after 30s"),
			expectedClass: azurerm.ErrorClassTransient,
		},
		{
			name:          "throttled request",
			error:         errors.New("request throttled: too many requests"),
			expectedClass: azurerm.ErrorClassTransient,
		},
		{
			name:          "HTTP 429 too many requests",
			error:         errors.New("HTTP 429: too many requests"),
			expectedClass: azurerm.ErrorClassTransient,
		},
		{
			name:          "HTTP 500 internal server error",
			error:         errors.New("HTTP 500: internal server error"),
			expectedClass: azurerm.ErrorClassTransient,
		},
		{
			name:          "HTTP 503 service unavailable",
			error:         errors.New("HTTP 503: service unavailable"),
			expectedClass: azurerm.ErrorClassTransient,
		},
		{
			name:          "permission denied",
			error:         errors.New("permission denied: insufficient access"),
			expectedClass: azurerm.ErrorClassPermissions,
		},
		{
			name:          "access denied error",
			error:         errors.New("access denied to resource"),
			expectedClass: azurerm.ErrorClassPermissions,
		},
		{
			name:          "quota exceeded error",
			error:         errors.New("quota exceeded for resource type"),
			expectedClass: azurerm.ErrorClassQuotaLimits,
		},
		{
			name:          "limit exceeded error",
			error:         errors.New("limit exceeded: maximum number reached"),
			expectedClass: azurerm.ErrorClassQuotaLimits,
		},
		{
			name:          "generic error defaults to user input",
			error:         errors.New("some unclassified error"),
			expectedClass: azurerm.ErrorClassUserInput,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurerm.ClassifyError(tc.error)
			assert.Equal(t, tc.expectedClass, result)
		})
	}
}

func TestMaskSubscriptionID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard subscription ID",
			input:    "12345678-1234-1234-1234-123456789012",
			expected: "1234****9012",
		},
		{
			name:     "simple 32 character ID",
			input:    "12345678901234567890123456789012",
			expected: "1234****9012",
		},
		{
			name:     "short ID returns masked placeholder",
			input:    "short",
			expected: "****",
		},
		{
			name:     "empty string returns masked placeholder",
			input:    "",
			expected: "****",
		},
		{
			name:     "minimum valid length (8 chars)",
			input:    "12345678",
			expected: "1234****5678",
		},
		{
			name:     "exactly 7 characters (too short)",
			input:    "1234567",
			expected: "****",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurerm.MaskSubscriptionID(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNewAzureTelemetryCollector(t *testing.T) {
	t.Parallel()

	logger := log.New()

	// Test with nil telemeter
	collector := azurerm.NewAzureTelemetryCollector(nil, logger)
	require.NotNil(t, collector)
	// Note: Cannot test private fields from external package
	// Test functionality instead through public methods

	// Test with valid telemeter would require more complex setup
	// For now, focus on nil safety which is the most common case during backend creation
}

func TestAzureTelemetryCollector_NilSafety(t *testing.T) {
	t.Parallel()

	logger := log.New()

	// Test with nil telemeter - should not panic
	collector := azurerm.NewAzureTelemetryCollector(nil, logger)
	require.NotNil(t, collector)

	ctx := t.Context()
	err := errors.New("test error")

	// These should not panic with nil telemeter
	require.NotPanics(t, func() {
		collector.LogError(ctx, err, azurerm.OperationBootstrap, azurerm.AzureErrorMetrics{
			ErrorType:      "TestError",
			Classification: azurerm.ErrorClassConfiguration,
		})
	})

	require.NotPanics(t, func() {
		collector.LogOperation(ctx, azurerm.OperationBootstrap, time.Second, map[string]interface{}{
			"test": "value",
		})
	})
}

func TestAzureTelemetryCollector_LogError(t *testing.T) {
	t.Parallel()

	logger := log.New()
	collector := azurerm.NewAzureTelemetryCollector(nil, logger)

	ctx := t.Context()
	err := errors.New("test authentication error")

	// Test with minimal metrics
	collector.LogError(ctx, err, azurerm.OperationBootstrap, azurerm.AzureErrorMetrics{
		ErrorType: "AuthError",
	})

	// Test with comprehensive metrics
	collector.LogError(ctx, err, azurerm.OperationBootstrap, azurerm.AzureErrorMetrics{
		ErrorType:      "AuthError",
		Classification: azurerm.ErrorClassAuthentication,
		Operation:      azurerm.OperationBootstrap,
		ResourceType:   "storage_account",
		ResourceName:   "testaccount",
		SubscriptionID: "12345678-1234-1234-1234-123456789012",
		Location:       "eastus",
		AuthMethod:     "azuread",
		StatusCode:     401,
		RetryAttempts:  2,
		Duration:       time.Second * 5,
		IsRetryable:    false,
		Additional: map[string]interface{}{
			"extra_info": "test_value",
		},
	})

	// Test with nil error - should return early
	collector.LogError(ctx, nil, azurerm.OperationBootstrap, azurerm.AzureErrorMetrics{
		ErrorType: "TestError",
	})
}

func TestAzureTelemetryCollector_LogOperation(t *testing.T) {
	t.Parallel()

	logger := log.New()
	collector := azurerm.NewAzureTelemetryCollector(nil, logger)

	ctx := t.Context()

	// Test successful operation logging
	collector.LogOperation(ctx, azurerm.OperationBootstrap, time.Second*2, map[string]interface{}{
		"storage_account": "testaccount",
		"container":       "testcontainer",
		"status":          "completed",
	})

	// Test with nil attributes
	collector.LogOperation(ctx, azurerm.OperationDelete, time.Millisecond*500, nil)

	// Test with empty attributes
	collector.LogOperation(ctx, azurerm.OperationValidation, time.Millisecond*100, map[string]interface{}{})
}

func TestIsSensitiveAttribute(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		key             string
		expectSensitive bool
	}{
		{"subscription ID is sensitive", "subscription_id", true},
		{"client secret is sensitive", "client_secret", true},
		{"client ID is sensitive", "client_id", true},
		{"tenant ID is sensitive", "tenant_id", true},
		{"password is sensitive", "password", true},
		{"access key is sensitive", "access_key", true},
		{"sas token is sensitive", "sas_token", true},
		{"connection string is sensitive", "connection_string", true},
		{"operation type is not sensitive", "operation_type", false},
		{"duration is not sensitive", "duration_ms", false},
		{"status is not sensitive", "status", false},
		{"resource type is not sensitive", "resource_type", false},
		{"location is not sensitive", "location", false},
		{"case insensitive detection", "SUBSCRIPTION_ID", true},
		{"case insensitive client secret", "CLIENT_SECRET", true},
		{"partial match in longer key", "my_password_value", true},
		{"non-sensitive with sensitive substring", "operation", false},
		{"generic key is not sensitive", "key", false},
		{"generic token is not sensitive", "token", false},
		{"generic secret is not sensitive", "secret", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurerm.IsSensitiveAttribute(tc.key)
			assert.Equal(t, tc.expectSensitive, result)
		})
	}
}

func TestFormatLogMessage(t *testing.T) {
	t.Parallel()

	metrics := azurerm.AzureErrorMetrics{
		ErrorType:    "ConfigError",
		Operation:    azurerm.OperationBootstrap,
		ResourceType: "storage_account",
		ResourceName: "testaccount",
		StatusCode:   404,
		ErrorMessage: "Configuration error occurred",
	}

	fields := map[string]interface{}{
		"error_type":     "ConfigError",
		"operation":      "bootstrap",
		"resource_type":  "storage_account",
		"resource_name":  "testaccount",
		"status_code":    404,
		"retry_attempts": 0,
	}

	result := azurerm.FormatLogMessage(metrics, fields)

	// Should contain the base error message and structured parts in brackets
	assert.Contains(t, result, "Configuration error occurred")
	assert.Contains(t, result, "[operation=bootstrap")
	assert.Contains(t, result, "resource=storage_account/testaccount")
	assert.Contains(t, result, "status=404")

	// Test with minimal metrics
	minimalMetrics := azurerm.AzureErrorMetrics{
		ErrorType:    "TestError",
		Operation:    azurerm.OperationDelete,
		ErrorMessage: "Test error message",
	}

	minimalFields := map[string]interface{}{
		"error_type": "TestError",
		"operation":  "delete",
	}

	minimalResult := azurerm.FormatLogMessage(minimalMetrics, minimalFields)
	assert.Contains(t, minimalResult, "Test error message")
	assert.Contains(t, minimalResult, "[operation=delete]")
}

func TestFormatSuccessMessage(t *testing.T) {
	t.Parallel()

	fields := map[string]interface{}{
		"operation":       "bootstrap",
		"duration_ms":     int64(1500),
		"storage_account": "testaccount",
		"container":       "testcontainer",
		"status":          "completed",
	}

	result := azurerm.FormatSuccessMessage(azurerm.OperationBootstrap, fields)

	// Should contain operation and duration formatting
	assert.Contains(t, result, "operation=bootstrap")
	assert.Contains(t, result, "[duration=1500ms]")

	// Test with minimal fields
	minimalFields := map[string]interface{}{
		"operation":   "delete",
		"duration_ms": int64(500),
	}

	minimalResult := azurerm.FormatSuccessMessage(azurerm.OperationDelete, minimalFields)
	assert.Contains(t, minimalResult, "operation=delete")
	assert.Contains(t, minimalResult, "[duration=500ms]")

	// Test without duration
	noDurationFields := map[string]interface{}{
		"operation": "validation",
		"status":    "completed",
	}

	noDurationResult := azurerm.FormatSuccessMessage(azurerm.OperationValidation, noDurationFields)
	assert.Contains(t, noDurationResult, "operation=validation")
	// Should not have brackets when no duration
	assert.NotContains(t, noDurationResult, "[")
}

func TestAzureErrorMetrics_ComprehensiveFields(t *testing.T) {
	t.Parallel()

	// Test that azurerm.AzureErrorMetrics can hold all expected field types
	metrics := azurerm.AzureErrorMetrics{
		ErrorType:      "TestError",
		Classification: azurerm.ErrorClassAuthentication,
		Operation:      azurerm.OperationBootstrap,
		ResourceType:   "storage_account",
		ResourceName:   "testaccount",
		SubscriptionID: "test-subscription",
		Location:       "eastus",
		AuthMethod:     "azuread",
		StatusCode:     401,
		RetryAttempts:  3,
		Duration:       time.Second * 10,
		IsRetryable:    true,
		ErrorMessage:   "Authentication failed",
		StackTrace:     "stack trace here",
		Additional: map[string]interface{}{
			"custom_field": "custom_value",
			"numeric":      42,
		},
	}

	// Verify all fields can be set and accessed
	assert.Equal(t, "TestError", metrics.ErrorType)
	assert.Equal(t, azurerm.ErrorClassAuthentication, metrics.Classification)
	assert.Equal(t, azurerm.OperationBootstrap, metrics.Operation)
	assert.Equal(t, "storage_account", metrics.ResourceType)
	assert.Equal(t, "testaccount", metrics.ResourceName)
	assert.Equal(t, "test-subscription", metrics.SubscriptionID)
	assert.Equal(t, "eastus", metrics.Location)
	assert.Equal(t, "azuread", metrics.AuthMethod)
	assert.Equal(t, 401, metrics.StatusCode)
	assert.Equal(t, 3, metrics.RetryAttempts)
	assert.Equal(t, time.Second*10, metrics.Duration)
	assert.True(t, metrics.IsRetryable)
	assert.Equal(t, "Authentication failed", metrics.ErrorMessage)
	assert.Equal(t, "stack trace here", metrics.StackTrace)
	assert.Equal(t, "custom_value", metrics.Additional["custom_field"])
	assert.Equal(t, 42, metrics.Additional["numeric"])
}
