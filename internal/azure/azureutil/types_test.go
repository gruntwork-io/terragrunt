package azureutil_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureutil"
	"github.com/stretchr/testify/assert"
)

// TestOperationType tests the OperationType constants and string conversion
func TestOperationType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation azureutil.OperationType
		expected  string
	}{
		{
			name:      "bootstrap operation",
			operation: azureutil.OperationBootstrap,
			expected:  "bootstrap",
		},
		{
			name:      "needs bootstrap operation",
			operation: azureutil.OperationNeedsBootstrap,
			expected:  "needs_bootstrap",
		},
		{
			name:      "delete operation",
			operation: azureutil.OperationDelete,
			expected:  "delete",
		},
		{
			name:      "delete container operation",
			operation: azureutil.OperationDeleteContainer,
			expected:  "delete_container",
		},
		{
			name:      "delete account operation",
			operation: azureutil.OperationDeleteAccount,
			expected:  "delete_account",
		},
		{
			name:      "migrate operation",
			operation: azureutil.OperationMigrate,
			expected:  "migrate",
		},
		{
			name:      "container operation",
			operation: azureutil.OperationContainerOp,
			expected:  "container_operation",
		},
		{
			name:      "storage operation",
			operation: azureutil.OperationStorageOp,
			expected:  "storage_operation",
		},
		{
			name:      "validation operation",
			operation: azureutil.OperationValidation,
			expected:  "validation",
		},
		{
			name:      "authentication operation",
			operation: azureutil.OperationAuthentication,
			expected:  "authentication",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := string(tc.operation)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestErrorClassConstants tests the ErrorClass constants
func TestErrorClassConstants(t *testing.T) {
	t.Parallel()

	// Test that constants exist and have expected values
	assert.Equal(t, "authorization", string(azureutil.ErrorClassAuthorization))
	assert.Equal(t, "permission", string(azureutil.ErrorClassPermission))
	assert.Equal(t, "invalid_request", string(azureutil.ErrorClassInvalidRequest))
	assert.Equal(t, "resource", string(azureutil.ErrorClassResource))
	assert.Equal(t, "networking", string(azureutil.ErrorClassNetworking))
	assert.Equal(t, "not_found", string(azureutil.ErrorClassNotFound))
	assert.Equal(t, "throttling", string(azureutil.ErrorClassThrottling))
	assert.Equal(t, "system", string(azureutil.ErrorClassSystem))
	assert.Equal(t, "unknown", string(azureutil.ErrorClassUnknown))
}

// TestClassifyError tests the error classification function
func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected azureutil.ErrorClass
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: azureutil.ErrorClassUnknown,
		},
		{
			name:     "authentication error - unauthorized",
			err:      errors.New("unauthorized access"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "authentication error - auth failed",
			err:      errors.New("authentication failed"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "authentication error - permission denied",
			err:      errors.New("permission denied"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "authentication error - access denied",
			err:      errors.New("access denied"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "permission error - rbac",
			err:      errors.New("RBAC permission required"),
			expected: azureutil.ErrorClassPermission,
		},
		{
			name:     "permission error - role assignment",
			err:      errors.New("role assignment failed"),
			expected: azureutil.ErrorClassPermission,
		},
		{
			name:     "invalid request - config error",
			err:      errors.New("invalid config parameter"),
			expected: azureutil.ErrorClassInvalidRequest,
		},
		{
			name:     "invalid request - argument error",
			err:      errors.New("missing required argument"),
			expected: azureutil.ErrorClassInvalidRequest,
		},
		{
			name:     "resource error - container",
			err:      errors.New("container does not exist"),
			expected: azureutil.ErrorClassResource,
		},
		{
			name:     "resource error - storage account",
			err:      errors.New("storage account not found"),
			expected: azureutil.ErrorClassResource,
		},
		{
			name:     "resource error - blob",
			err:      errors.New("blob operation failed"),
			expected: azureutil.ErrorClassResource,
		},
		{
			name:     "networking error - connection",
			err:      errors.New("connection failed"),
			expected: azureutil.ErrorClassNetworking,
		},
		{
			name:     "networking error - tcp",
			err:      errors.New("TCP connection timeout"),
			expected: azureutil.ErrorClassNetworking,
		},
		{
			name:     "networking error - http",
			err:      errors.New("HTTP request failed"),
			expected: azureutil.ErrorClassNetworking,
		},
		{
			name:     "not found error - 404",
			err:      errors.New("404 not found"),
			expected: azureutil.ErrorClassNotFound,
		},
		{
			name:     "not found error - does not exist",
			err:      errors.New("resource does not exist"),
			expected: azureutil.ErrorClassNotFound,
		},
		{
			name:     "throttling error - quota exceeded",
			err:      errors.New("quota exceeded"),
			expected: azureutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - rate limit",
			err:      errors.New("rate limit exceeded"),
			expected: azureutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - 429",
			err:      errors.New("429 too many requests"),
			expected: azureutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - throttled",
			err:      errors.New("request throttled"),
			expected: azureutil.ErrorClassThrottling,
		},
		{
			name:     "system error - internal server error",
			err:      errors.New("internal server error"),
			expected: azureutil.ErrorClassSystem,
		},
		{
			name:     "system error - system failure",
			err:      errors.New("system failure"),
			expected: azureutil.ErrorClassSystem,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error message"),
			expected: azureutil.ErrorClassUnknown,
		},
		{
			name:     "case insensitive matching",
			err:      errors.New("AUTHENTICATION FAILED"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "multiple keywords - first match wins",
			err:      errors.New("authentication failed during container operation"),
			expected: azureutil.ErrorClassAuthorization,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azureutil.ClassifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestErrorMetrics tests the ErrorMetrics struct
func TestErrorMetrics(t *testing.T) {
	t.Parallel()

	metrics := azureutil.ErrorMetrics{
		ErrorType:      "AuthenticationError",
		Classification: azureutil.ErrorClassAuthorization,
		Operation:      azureutil.OperationAuthentication,
		ResourceType:   "StorageAccount",
		ResourceName:   "teststorage",
		ErrorMessage:   "authentication failed",
		IsRetryable:    false,
		SubscriptionID: "sub-12345",
		Location:       "eastus",
		AuthMethod:     "service-principal",
		StatusCode:     401,
		RetryAttempts:  0,
		Additional:     map[string]interface{}{"test": "value"},
	}

	assert.Equal(t, "AuthenticationError", metrics.ErrorType)
	assert.Equal(t, azureutil.ErrorClassAuthorization, metrics.Classification)
	assert.Equal(t, azureutil.OperationAuthentication, metrics.Operation)
	assert.Equal(t, "StorageAccount", metrics.ResourceType)
	assert.Equal(t, "teststorage", metrics.ResourceName)
	assert.Equal(t, "authentication failed", metrics.ErrorMessage)
	assert.False(t, metrics.IsRetryable)
	assert.Equal(t, "sub-12345", metrics.SubscriptionID)
	assert.Equal(t, "eastus", metrics.Location)
	assert.Equal(t, "service-principal", metrics.AuthMethod)
	assert.Equal(t, 401, metrics.StatusCode)
	assert.Equal(t, 0, metrics.RetryAttempts)
	assert.Equal(t, map[string]interface{}{"test": "value"}, metrics.Additional)
}

// TestStructDefaults tests default values for structs
func TestStructDefaults(t *testing.T) {
	t.Parallel()

	t.Run("ErrorMetrics defaults", func(t *testing.T) {
		t.Parallel()

		metrics := azureutil.ErrorMetrics{}

		assert.Empty(t, metrics.ErrorType)
		assert.Equal(t, azureutil.ErrorClass(""), metrics.Classification)
		assert.Equal(t, azureutil.OperationType(""), metrics.Operation)
		assert.Empty(t, metrics.ResourceType)
		assert.Empty(t, metrics.ResourceName)
		assert.Empty(t, metrics.ErrorMessage)
		assert.False(t, metrics.IsRetryable)
		assert.Empty(t, metrics.SubscriptionID)
		assert.Empty(t, metrics.Location)
		assert.Empty(t, metrics.AuthMethod)
		assert.Equal(t, 0, metrics.StatusCode)
		assert.Equal(t, 0, metrics.RetryAttempts)
		assert.Nil(t, metrics.Additional)
	})
}

// TestStructFieldAssignment tests that all struct fields can be assigned
func TestStructFieldAssignment(t *testing.T) {
	t.Parallel()

	t.Run("ErrorMetrics field assignment", func(t *testing.T) {
		t.Parallel()

		metrics := &azureutil.ErrorMetrics{}

		// Test field assignment
		metrics.ErrorType = "TestError"
		metrics.Classification = azureutil.ErrorClassNetworking
		metrics.Operation = azureutil.OperationStorageOp
		metrics.ResourceType = "BlobContainer"
		metrics.ResourceName = "testcontainer"
		metrics.ErrorMessage = "network timeout"
		metrics.IsRetryable = true
		metrics.SubscriptionID = "sub-test"
		metrics.Location = "westus"
		metrics.AuthMethod = "managed-identity"
		metrics.StatusCode = 500
		metrics.RetryAttempts = 3
		metrics.Additional = map[string]interface{}{"context": "test"}

		// Verify assignments
		assert.Equal(t, "TestError", metrics.ErrorType)
		assert.Equal(t, azureutil.ErrorClassNetworking, metrics.Classification)
		assert.Equal(t, azureutil.OperationStorageOp, metrics.Operation)
		assert.Equal(t, "BlobContainer", metrics.ResourceType)
		assert.Equal(t, "testcontainer", metrics.ResourceName)
		assert.Equal(t, "network timeout", metrics.ErrorMessage)
		assert.True(t, metrics.IsRetryable)
		assert.Equal(t, "sub-test", metrics.SubscriptionID)
		assert.Equal(t, "westus", metrics.Location)
		assert.Equal(t, "managed-identity", metrics.AuthMethod)
		assert.Equal(t, 500, metrics.StatusCode)
		assert.Equal(t, 3, metrics.RetryAttempts)
		assert.Equal(t, map[string]interface{}{"context": "test"}, metrics.Additional)
	})
}

// TestErrorClassClassification tests error classification edge cases
func TestErrorClassClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected azureutil.ErrorClass
	}{
		{
			name:     "empty error message",
			err:      errors.New(""),
			expected: azureutil.ErrorClassUnknown,
		},
		{
			name:     "whitespace only error",
			err:      errors.New("   "),
			expected: azureutil.ErrorClassUnknown,
		},
		{
			name:     "mixed case keywords",
			err:      errors.New("Authentication Error occurred"),
			expected: azureutil.ErrorClassAuthorization,
		},
		{
			name:     "partial keyword match",
			err:      errors.New("authorize the request"),
			expected: azureutil.ErrorClassAuthorization, // "auth" is found in "authorize"
		},
		{
			name:     "keyword in middle of word",
			err:      errors.New("unauthenticated user"),
			expected: azureutil.ErrorClassAuthorization, // "auth" is found in "unauthenticated"
		},
		{
			name:     "multiple error classes - priority test",
			err:      errors.New("unauthorized container access"),
			expected: azureutil.ErrorClassAuthorization, // First match should win
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azureutil.ClassifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
