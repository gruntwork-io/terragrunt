//nolint:testpackage // Requires access to internal helpers for comprehensive coverage.
package azureutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTelemetryCollector implements the core functionality of TelemetryCollector for testing
type MockTelemetryCollector struct { //nolint:govet // test helper prioritizes readability over packing
	ErrorsLogged     int
	OperationsLogged int
	LastErrorMetrics ErrorMetrics
	LastOperation    OperationType
}

type testContextKey string

func (m *MockTelemetryCollector) LogError(ctx context.Context, err error, operation OperationType, metrics ErrorMetrics) {
	m.ErrorsLogged++
	m.LastErrorMetrics = metrics
}

func (m *MockTelemetryCollector) LogOperation(ctx context.Context, operation OperationType, duration time.Duration, attrs map[string]interface{}) {
	m.OperationsLogged++
	m.LastOperation = operation
}

func TestWithErrorHandling_Success(t *testing.T) {
	t.Parallel()

	// Set up
	collector := &MockTelemetryCollector{}

	var logger log.Logger // Using a nil logger for tests

	handler := NewErrorHandler(collector, logger)
	ctx := context.Background()

	// Test with a successful operation
	err := handler.WithErrorHandling(
		ctx,
		OperationBootstrap,
		"storage_account",
		"teststorage",
		func() error {
			return nil // Success case
		},
	)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 0, collector.ErrorsLogged)
	assert.Equal(t, 1, collector.OperationsLogged)
	assert.Equal(t, OperationBootstrap, collector.LastOperation)
}

func TestWithErrorHandling_Error(t *testing.T) {
	t.Parallel()

	// Set up
	collector := &MockTelemetryCollector{}

	var logger log.Logger // Using a nil logger for tests

	handler := NewErrorHandler(collector, logger)
	ctx := context.Background()
	testError := errors.New("test error")

	// Test with a failed operation
	err := handler.WithErrorHandling(
		ctx,
		OperationBootstrap,
		"storage_account",
		"teststorage",
		func() error {
			return testError
		},
	)

	// Verify
	require.Error(t, err)
	assert.Equal(t, 1, collector.ErrorsLogged)
	assert.Equal(t, 0, collector.OperationsLogged)
	assert.Equal(t, "storage_account", collector.LastErrorMetrics.ResourceType)
	assert.Equal(t, "teststorage", collector.LastErrorMetrics.ResourceName)
	assert.Equal(t, OperationBootstrap, collector.LastErrorMetrics.Operation)
}

func TestErrorTypeDetection(t *testing.T) {
	t.Parallel()

	handler := NewErrorHandler(nil, nil)

	testCases := []struct { //nolint:govet // test table structure simplicity prioritized
		errMsg       string
		expectedType string
	}{
		{"unauthorized access", "AuthenticationError"},
		{"permission denied", "AuthenticationError"},
		{"resource not found", "NotFoundError"},
		{"container does not exist", "NotFoundError"},
		{"already exists", "ConflictError"},
		{"validation failed", "ValidationError"},
		{"request timed out", "TimeoutError"},
		{"rate limit exceeded", "ThrottlingError"},
		{"network connection failed", "NetworkError"},
		{"something else", "UnknownError"},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.errMsg, func(t *testing.T) {
			t.Parallel()

			err := errors.New(tc.errMsg)
			errorType := handler.determineErrorType(err)
			assert.Equal(t, tc.expectedType, errorType)
		})
	}
}

func TestRetryableErrorDetection(t *testing.T) {
	t.Parallel()

	handler := NewErrorHandler(nil, nil)

	testCases := []struct { //nolint:govet // prioritizing readability in table layout
		errMsg      string
		isRetryable bool
	}{
		{"timeout occurred", true},
		{"request timed out", true},
		{"rate limit exceeded", true},
		{"throttling error", true},
		{"service unavailable", true},
		{"connection reset", true},
		{"try again", true},
		{"resource not found", false},
		{"invalid parameter", false},
		{"access denied", false},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.errMsg, func(t *testing.T) {
			t.Parallel()

			err := errors.New(tc.errMsg)
			isRetryable := handler.isRetryableError(err)
			assert.Equal(t, tc.isRetryable, isRetryable)
		})
	}
}

// Test the WithAuthErrorHandling function
func TestWithAuthErrorHandling(t *testing.T) {
	t.Parallel()

	logger := log.New()
	collector := &MockTelemetryCollector{}
	handler := NewErrorHandler(collector, logger)
	ctx := context.Background()

	// Create a mock auth config
	authConfig := &azureauth.AuthConfig{
		Method:         "azuread",
		SubscriptionID: "test-subscription-id",
		TenantID:       "test-tenant-id",
	}

	// Test successful operation
	err := handler.WithAuthErrorHandling(ctx, authConfig, func() error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, collector.OperationsLogged)
	assert.Equal(t, 0, collector.ErrorsLogged)
	assert.Equal(t, OperationAuthentication, collector.LastOperation)

	// Test failed operation
	testErr := errors.New("authentication failed: invalid credentials")
	err = handler.WithAuthErrorHandling(ctx, authConfig, func() error {
		return testErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Azure authentication failed using")
	assert.Contains(t, err.Error(), "azuread")
	assert.Contains(t, err.Error(), "invalid credentials")
	assert.Equal(t, 1, collector.OperationsLogged)
	assert.Equal(t, 1, collector.ErrorsLogged)
	assert.Equal(t, OperationAuthentication, collector.LastErrorMetrics.Operation)
	assert.Equal(t, "authorization", string(collector.LastErrorMetrics.Classification))
	assert.Equal(t, "authentication", collector.LastErrorMetrics.ResourceType)
	assert.Equal(t, "azuread", collector.LastErrorMetrics.ResourceName)
}

// TestErrorClassificationComprehensive tests detailed error classification
func TestErrorClassificationComprehensive(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // test table readability over packing
		name          string
		errMsg        string
		expectedClass ErrorClass
	}{
		// Permission errors (following original types_test.go expectations)
		{"unauthorized", "unauthorized access", ErrorClassAuthorization},
		{"forbidden", "forbidden operation", ErrorClassUnknown}, // No specific match in original logic
		{"access denied", "access denied to resource", ErrorClassAuthorization},
		{"insufficient privileges", "insufficient privileges", ErrorClassPermission},

		// Authentication errors
		{"auth failed", "authentication failed", ErrorClassAuthorization},
		{"invalid credentials", "invalid credentials provided", ErrorClassAuthorization},
		{"token expired", "authentication token expired", ErrorClassAuthorization},

		// Not found errors (but resource-specific errors go to Resource class)
		{"not found", "resource not found", ErrorClassNotFound},
		{"does not exist", "container does not exist", ErrorClassResource},           // Resource class takes precedence
		{"missing resource", "the specified resource is missing", ErrorClassUnknown}, // No specific match

		// Throttling errors
		{"too many requests", "too many requests", ErrorClassThrottling},
		{"rate limit", "rate limit exceeded", ErrorClassThrottling},
		{"quota exceeded", "quota exceeded", ErrorClassThrottling},

		// Network errors
		{"connection failed", "network connection failed", ErrorClassNetworking},
		{"timeout", "operation timed out", ErrorClassNetworking},
		{"dns error", "dns resolution failed", ErrorClassNetworking},

		// Configuration errors
		{"invalid config", "invalid configuration", ErrorClassInvalidRequest},
		{"missing parameter", "required parameter missing", ErrorClassInvalidRequest},
		{"malformed", "malformed configuration file", ErrorClassInvalidRequest}, // Contains "config" so matches InvalidRequest

		// Validation errors
		{"validation failed", "validation failed", ErrorClassUnknown},  // No specific match in original logic
		{"bad request", "bad request format", ErrorClassUnknown},       // No specific match
		{"invalid input", "invalid input provided", ErrorClassUnknown}, // No specific match

		// Generic/unknown errors
		{"generic error", "something went wrong", ErrorClassUnknown},
		{"empty error", "", ErrorClassUnknown}, // Empty error message creates nil error, should return unknown
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err error
			if tc.errMsg == "" {
				err = nil
			} else {
				err = errors.New(tc.errMsg)
			}

			result := ClassifyError(err)
			assert.Equal(t, tc.expectedClass, result,
				"Error message '%s' should be classified as '%s', got '%s'",
				tc.errMsg, tc.expectedClass, result)
		})
	}
}

// TestErrorHandlerWithNilComponents tests error handler behavior with nil components
func TestErrorHandlerWithNilComponents(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // table layout chosen for clarity in tests
		name      string
		telemetry TelemetryCollector
		logger    log.Logger
	}{
		{
			name:      "nil telemetry",
			telemetry: nil,
			logger:    log.Default(),
		},
		{
			name:      "nil logger",
			telemetry: &MockTelemetryCollector{},
			logger:    nil,
		},
		{
			name:      "both nil",
			telemetry: nil,
			logger:    nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Should not panic when creating handler with nil components
			assert.NotPanics(t, func() {
				handler := NewErrorHandler(tc.telemetry, tc.logger)
				assert.NotNil(t, handler)
			})
		})
	}
}

// TestErrorMetricsFieldAccess tests that all ErrorMetrics fields are accessible
func TestErrorMetricsFieldAccess(t *testing.T) {
	t.Parallel()

	metrics := ErrorMetrics{
		ErrorType:      "test_error",
		Classification: ErrorClassPermission,
		Operation:      OperationBootstrap,
		ResourceType:   "storage_account",
		ResourceName:   "test-account",
		ErrorMessage:   "Test error message",
		IsRetryable:    true,
	}

	// Test all field access without panics
	assert.NotPanics(t, func() {
		_ = metrics.ErrorType
		_ = metrics.Classification
		_ = metrics.Operation
		_ = metrics.ResourceType
		_ = metrics.ResourceName
		_ = metrics.ErrorMessage
		_ = metrics.IsRetryable
	})

	// Test string conversions
	assert.Equal(t, "permission", string(metrics.Classification))
	assert.Equal(t, "bootstrap", string(metrics.Operation))
}

// TestOperationTypeStringConversion tests operation type string conversion
func TestOperationTypeStringConversion(t *testing.T) {
	t.Parallel()

	operations := map[OperationType]string{
		OperationBootstrap:       "bootstrap",
		OperationNeedsBootstrap:  "needs_bootstrap",
		OperationDelete:          "delete",
		OperationDeleteContainer: "delete_container",
		OperationDeleteAccount:   "delete_account",
		OperationMigrate:         "migrate",
	}

	for op, expected := range operations {
		op := op
		expected := expected
		t.Run(expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, string(op))

			// Test round-trip conversion
			converted := OperationType(expected)
			assert.Equal(t, op, converted)
		})
	}
}

// TestResourceTypeConstants tests resource type constants
func TestResourceTypeConstants(t *testing.T) {
	t.Parallel()

	expectedTypes := map[string]string{
		ResourceTypeBlob:          "blob",
		ResourceTypeContainer:     "container",
		ResourceTypeResourceGroup: "resource_group",
		ResourceTypeStorage:       "storage_account",
	}

	for constant, expected := range expectedTypes {
		constant := constant
		expected := expected
		t.Run(expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, constant)
			assert.NotEmpty(t, constant)
		})
	}
}

// TestIsPermissionErrorEdgeCases tests IsPermissionError with edge cases
func TestIsPermissionErrorEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // maintain descriptive test cases order
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "empty error message",
			err:      errors.New(""),
			expected: false,
		},
		{
			name:     "case insensitive - UNAUTHORIZED",
			err:      errors.New("UNAUTHORIZED"),
			expected: true,
		},
		{
			name:     "case insensitive - Forbidden",
			err:      errors.New("Forbidden"),
			expected: true,
		},
		{
			name:     "substring match - contains unauthorized",
			err:      errors.New("Request failed: unauthorized access to resource"),
			expected: true,
		},
		{
			name:     "substring match - contains permission",
			err:      errors.New("User lacks permission to perform this action"),
			expected: true,
		},
		{
			name:     "no match - generic error",
			err:      errors.New("generic failure occurred"),
			expected: false,
		},
		{
			name:     "no match - network error",
			err:      errors.New("network connection timeout"),
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := IsPermissionError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestErrorHandlerContextPropagation tests that context is properly propagated
func TestErrorHandlerContextPropagation(t *testing.T) {
	t.Parallel()

	collector := &MockTelemetryCollector{}
	handler := NewErrorHandler(collector, nil)

	tests := []struct { //nolint:govet // structured for readability in test coverage
		name string
		ctx  context.Context
	}{
		{
			name: "background context",
			ctx:  context.Background(),
		},
		{
			name: "context with timeout",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				return ctx
			}(),
		},
		{
			name: "context with values",
			ctx:  context.WithValue(context.Background(), testContextKey("key"), "value"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that context is handled without panics
			assert.NotPanics(t, func() {
				err := handler.WithErrorHandling(
					tc.ctx,
					OperationBootstrap,
					"test_resource",
					"test_name",
					func() error { return nil },
				)
				assert.NoError(t, err)
			})
		})
	}
}

// TestErrorHandlerLargeErrorMessage tests handling of large error messages
func TestErrorHandlerLargeErrorMessage(t *testing.T) {
	t.Parallel()

	collector := &MockTelemetryCollector{}
	handler := NewErrorHandler(collector, nil)
	ctx := context.Background()

	// Create a large error message
	largeMessage := strings.Repeat("This is a very long error message. ", 1000)
	largeErr := errors.New(largeMessage)

	// Test that large errors are handled gracefully
	assert.NotPanics(t, func() {
		err := handler.WithErrorHandling(
			ctx,
			OperationBootstrap,
			"test_resource",
			"test_name",
			func() error { return largeErr },
		)
		assert.Error(t, err)
	})

	// Verify error was logged
	assert.Equal(t, 1, collector.ErrorsLogged)
	assert.Contains(t, collector.LastErrorMetrics.ErrorMessage, "This is a very long error message")
}
