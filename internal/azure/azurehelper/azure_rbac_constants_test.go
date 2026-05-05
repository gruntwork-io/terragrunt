//go:build azure

package azurehelper_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/stretchr/testify/assert"
)

// TestRBACRetryConstants ensures the RBAC retry constants have the expected values
// for proper RBAC propagation handling (up to 5 minutes)
func TestRBACRetryConstants(t *testing.T) {
	t.Parallel()
	// Test RBAC delay is 10 seconds (longer delays for RBAC propagation)
	assert.Equal(t, 10*time.Second, azurehelper.RbacRetryDelay, "RBAC retry delay should be 10 seconds")

	// Test RBAC max retries is 30 (30 * 10s = 5 minutes max)
	assert.Equal(t, 30, azurehelper.RbacMaxRetries, "RBAC max retries should be 30")

	// Test that retry attempts equals max retries (simplified)
	assert.Equal(t, azurehelper.RbacMaxRetries, azurehelper.RbacRetryAttempts,
		"RBAC retry attempts should equal RbacMaxRetries")

	// Test the specific expected values
	assert.Equal(t, 30, azurehelper.RbacRetryAttempts, "RBAC retry attempts should be 30")

	// Test propagation timeout is 5 minutes
	assert.Equal(t, 5*time.Minute, azurehelper.RbacPropagationTimeout,
		"RBAC propagation timeout should be 5 minutes")

	// Verify the math: 30 attempts * 10s delay = 5 minutes max wait
	maxWaitTime := time.Duration(azurehelper.RbacRetryAttempts) * azurehelper.RbacRetryDelay
	assert.Equal(t, 5*time.Minute, maxWaitTime,
		"Max wait time should be 5 minutes (30 * 10s)")
}

// TestIsPermissionError tests the isPermissionError function with various error types
// nolint: govet
func TestIsPermissionError(t *testing.T) {
	t.Parallel()
	// Create storage account client for testing
	client := &azurehelper.StorageAccountClient{}

	// Test cases for various error messages
	testCases := []struct {
		name     string
		input    error
		expected bool
	}{
		{
			name:     "nil error",
			input:    nil,
			expected: false,
		},
		{
			name:     "basic authorization error",
			input:    errors.New("operation failed due to authorization failed"),
			expected: true,
		},
		{
			name:     "permission denied error",
			input:    errors.New("permission denied for operation"),
			expected: true,
		},
		{
			name:     "forbidden error",
			input:    errors.New("server returned status code 403 forbidden"),
			expected: true,
		},
		{
			name:     "access denied error",
			input:    errors.New("access denied to resource"),
			expected: true,
		},
		{
			name:     "not authorized error",
			input:    errors.New("client is not authorized to perform this action"),
			expected: true,
		},
		{
			name:     "role assignment error",
			input:    errors.New("waiting for role assignment to complete"),
			expected: true,
		},
		{
			name:     "storage blob data owner error",
			input:    errors.New("requires storage blob data owner role"),
			expected: true,
		},
		{
			name:     "unrelated error",
			input:    errors.New("connection timed out"),
			expected: false,
		},
		{
			name:     "network error",
			input:    errors.New("network connectivity issues"),
			expected: false,
		},
		{
			name:     "validation error",
			input:    errors.New("validation failed for resource"),
			expected: false,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := client.IsPermissionError(tc.input)
			assert.Equal(t, tc.expected, result,
				"IsPermissionError should return %v for error: %v", tc.expected, tc.input)
		})
	}

	// Test with wrapped errors
	t.Run("WrappedPermissionError", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("permission denied for storage account")
		wrappedErr := fmt.Errorf("operation failed: %w", innerErr)

		assert.True(t, client.IsPermissionError(wrappedErr),
			"Should detect permission error in wrapped error")
	})

	// Test with non-permission wrapped errors
	t.Run("WrappedNonPermissionError", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("resource not found")
		wrappedErr2 := fmt.Errorf("operation failed: %w", innerErr)

		assert.False(t, client.IsPermissionError(wrappedErr2),
			"Should not detect permission error in unrelated wrapped error")
	})
}
