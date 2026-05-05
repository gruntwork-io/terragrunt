package azureutil_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureutil"
	"github.com/gruntwork-io/terragrunt/internal/azure/errorutil"
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

// TestErrorClassConstants tests the ErrorClass constants from errorutil
func TestErrorClassConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "authentication", string(errorutil.ErrorClassAuthentication))
	assert.Equal(t, "authorization", string(errorutil.ErrorClassAuthorization))
	assert.Equal(t, "permission", string(errorutil.ErrorClassPermission))
	assert.Equal(t, "invalid_request", string(errorutil.ErrorClassInvalidRequest))
	assert.Equal(t, "resource", string(errorutil.ErrorClassResource))
	assert.Equal(t, "networking", string(errorutil.ErrorClassNetworking))
	assert.Equal(t, "not_found", string(errorutil.ErrorClassNotFound))
	assert.Equal(t, "throttling", string(errorutil.ErrorClassThrottling))
	assert.Equal(t, "transient", string(errorutil.ErrorClassTransient))
	assert.Equal(t, "configuration", string(errorutil.ErrorClassConfiguration))
	assert.Equal(t, "system", string(errorutil.ErrorClassSystem))
	assert.Equal(t, "unknown", string(errorutil.ErrorClassUnknown))
}

// TestClassifyError tests the error classification function
func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected errorutil.ErrorClass
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: errorutil.ErrorClassUnknown,
		},
		{
			name:     "authentication error - unauthorized",
			err:      errors.New("unauthorized access"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "authentication error - auth failed",
			err:      errors.New("authentication failed"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "authentication error - permission denied",
			err:      errors.New("permission denied"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "authentication error - access denied",
			err:      errors.New("access denied"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "permission error - rbac",
			err:      errors.New("RBAC permission required"),
			expected: errorutil.ErrorClassPermission,
		},
		{
			name:     "permission error - role assignment",
			err:      errors.New("role assignment failed"),
			expected: errorutil.ErrorClassPermission,
		},
		{
			name:     "invalid request - config error",
			err:      errors.New("invalid config parameter"),
			expected: errorutil.ErrorClassInvalidRequest,
		},
		{
			name:     "invalid request - argument error",
			err:      errors.New("missing required argument"),
			expected: errorutil.ErrorClassInvalidRequest,
		},
		{
			name:     "resource error - container",
			err:      errors.New("container does not exist"),
			expected: errorutil.ErrorClassResource,
		},
		{
			name:     "resource error - storage account",
			err:      errors.New("storage account not found"),
			expected: errorutil.ErrorClassResource,
		},
		{
			name:     "resource error - blob",
			err:      errors.New("blob operation failed"),
			expected: errorutil.ErrorClassResource,
		},
		{
			name:     "networking error - connection",
			err:      errors.New("connection failed"),
			expected: errorutil.ErrorClassNetworking,
		},
		{
			name:     "networking error - tcp",
			err:      errors.New("TCP connection timeout"),
			expected: errorutil.ErrorClassNetworking,
		},
		{
			name:     "networking error - http",
			err:      errors.New("HTTP request failed"),
			expected: errorutil.ErrorClassNetworking,
		},
		{
			name:     "not found error - 404",
			err:      errors.New("404 not found"),
			expected: errorutil.ErrorClassNotFound,
		},
		{
			name:     "not found error - does not exist",
			err:      errors.New("resource does not exist"),
			expected: errorutil.ErrorClassNotFound,
		},
		{
			name:     "throttling error - quota exceeded",
			err:      errors.New("quota exceeded"),
			expected: errorutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - rate limit",
			err:      errors.New("rate limit exceeded"),
			expected: errorutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - 429",
			err:      errors.New("429 too many requests"),
			expected: errorutil.ErrorClassThrottling,
		},
		{
			name:     "throttling error - throttled",
			err:      errors.New("request throttled"),
			expected: errorutil.ErrorClassThrottling,
		},
		{
			name:     "transient error - internal server error",
			err:      errors.New("internal server error"),
			expected: errorutil.ErrorClassTransient,
		},
		{
			name:     "transient error - system failure",
			err:      errors.New("system failure"),
			expected: errorutil.ErrorClassTransient,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error message"),
			expected: errorutil.ErrorClassUnknown,
		},
		{
			name:     "case insensitive matching",
			err:      errors.New("AUTHENTICATION FAILED"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "multiple keywords - first match wins",
			err:      errors.New("authentication failed during container operation"),
			expected: errorutil.ErrorClassAuthentication,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := errorutil.ClassifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestErrorMetrics tests the ErrorMetrics struct
func TestErrorMetrics(t *testing.T) {
	t.Parallel()

	metrics := azureutil.ErrorMetrics{
		ErrorType:      "AuthenticationError",
		Classification: errorutil.ErrorClassAuthorization,
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
	assert.Equal(t, errorutil.ErrorClassAuthorization, metrics.Classification)
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
		assert.Equal(t, errorutil.ErrorClass(""), metrics.Classification)
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

		metrics.ErrorType = "TestError"
		metrics.Classification = errorutil.ErrorClassNetworking
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

		assert.Equal(t, "TestError", metrics.ErrorType)
		assert.Equal(t, errorutil.ErrorClassNetworking, metrics.Classification)
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
		expected errorutil.ErrorClass
	}{
		{
			name:     "empty error message",
			err:      errors.New(""),
			expected: errorutil.ErrorClassUnknown,
		},
		{
			name:     "whitespace only error",
			err:      errors.New("   "),
			expected: errorutil.ErrorClassUnknown,
		},
		{
			name:     "mixed case keywords",
			err:      errors.New("Authentication Error occurred"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "partial keyword match",
			err:      errors.New("authorize the request"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "keyword in middle of word",
			err:      errors.New("unauthenticated user"),
			expected: errorutil.ErrorClassAuthentication,
		},
		{
			name:     "multiple error classes - priority test",
			err:      errors.New("unauthorized container access"),
			expected: errorutil.ErrorClassAuthentication,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := errorutil.ClassifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
