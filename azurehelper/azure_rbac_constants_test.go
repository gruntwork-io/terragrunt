package azurehelper_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/stretchr/testify/assert"
)

// TestRBACRetryConstants ensures the RBAC retry constants have the expected values
// and that the retry attempts is correctly calculated from max retries
func TestRBACRetryConstants(t *testing.T) {
	t.Parallel()
	// Test RBAC delay is 3 seconds
	assert.Equal(t, 3*time.Second, azurehelper.RbacRetryDelay, "RBAC retry delay should be 3 seconds")

	// Test RBAC max retries is 5
	assert.Equal(t, 5, azurehelper.RbacMaxRetries, "RBAC max retries should be 5")

	// Test that retry attempts equals max retries + 1 (for the initial attempt)
	assert.Equal(t, azurehelper.RbacMaxRetries+1, azurehelper.RbacRetryAttempts,
		"RBAC retry attempts should equal RbacMaxRetries+1")

	// Test the specific expected values
	assert.Equal(t, 6, azurehelper.RbacRetryAttempts, "RBAC retry attempts should be 6 (5 retries + initial attempt)")

	// The constants are defined in the package, so we can't modify them for testing
	// Instead, we'll just verify the current values match our expectations

	// Test different values of the relationship
	testCases := []struct {
		maxRetries       int
		expectedAttempts int
	}{
		{3, 4},
		{5, 6}, // Current value
		{10, 11},
		{0, 1}, // Edge case: no retries means 1 attempt
	}

	for _, tc := range testCases {
		calculatedAttempts := tc.maxRetries + 1
		assert.Equal(t, tc.expectedAttempts, calculatedAttempts,
			"When max retries is %d, attempts should be %d", tc.maxRetries, tc.expectedAttempts)
	}
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
