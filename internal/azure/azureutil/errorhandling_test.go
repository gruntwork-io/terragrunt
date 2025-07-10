package azureutil

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

// MockTelemetryCollector implements the core functionality of TelemetryCollector for testing
type MockTelemetryCollector struct {
	ErrorsLogged     int
	OperationsLogged int
	LastErrorMetrics ErrorMetrics
	LastOperation    OperationType
}

func (m *MockTelemetryCollector) LogError(ctx context.Context, err error, operation OperationType, metrics ErrorMetrics) {
	m.ErrorsLogged++
	m.LastErrorMetrics = metrics
}

func (m *MockTelemetryCollector) LogOperation(ctx context.Context, operation OperationType, duration time.Duration, attrs map[string]interface{}) {
	m.OperationsLogged++
	m.LastOperation = operation
}

func TestWithErrorHandling_Success(t *testing.T) {
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
	assert.NoError(t, err)
	assert.Equal(t, 0, collector.ErrorsLogged)
	assert.Equal(t, 1, collector.OperationsLogged)
	assert.Equal(t, OperationBootstrap, collector.LastOperation)
}

func TestWithErrorHandling_Error(t *testing.T) {
	// Set up
	collector := &MockTelemetryCollector{}
	var logger log.Logger // Using a nil logger for tests
	handler := NewErrorHandler(collector, logger)
	ctx := context.Background()
	testError := fmt.Errorf("test error")

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
	assert.Error(t, err)
	assert.Equal(t, 1, collector.ErrorsLogged)
	assert.Equal(t, 0, collector.OperationsLogged)
	assert.Equal(t, "storage_account", collector.LastErrorMetrics.ResourceType)
	assert.Equal(t, "teststorage", collector.LastErrorMetrics.ResourceName)
	assert.Equal(t, OperationBootstrap, collector.LastErrorMetrics.Operation)
}

func TestErrorTypeDetection(t *testing.T) {
	handler := NewErrorHandler(nil, nil)

	testCases := []struct {
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
		t.Run(tc.errMsg, func(t *testing.T) {
			err := fmt.Errorf("%s", tc.errMsg)
			errorType := handler.determineErrorType(err)
			assert.Equal(t, tc.expectedType, errorType)
		})
	}
}

func TestRetryableErrorDetection(t *testing.T) {
	handler := NewErrorHandler(nil, nil)

	testCases := []struct {
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
		t.Run(tc.errMsg, func(t *testing.T) {
			err := fmt.Errorf("%s", tc.errMsg)
			isRetryable := handler.isRetryableError(err)
			assert.Equal(t, tc.isRetryable, isRetryable)
		})
	}
}

// Test the WithAuthErrorHandling function
func TestWithAuthErrorHandling(t *testing.T) {
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
	assert.NoError(t, err)
	assert.Equal(t, 1, collector.OperationsLogged)
	assert.Equal(t, 0, collector.ErrorsLogged)
	assert.Equal(t, OperationAuthentication, collector.LastOperation)

	// Test failed operation
	testErr := fmt.Errorf("authentication failed: invalid credentials")
	err = handler.WithAuthErrorHandling(ctx, authConfig, func() error {
		return testErr
	})
	assert.Error(t, err)
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

	tests := []struct {
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
		{"empty error", "", ""}, // Empty error message actually creates nil error, which returns empty string
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err error
			if tc.errMsg == "" {
				err = nil
			} else {
				err = fmt.Errorf("%s", tc.errMsg)
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

	tests := []struct {
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

	tests := []struct {
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
			err:      fmt.Errorf(""),
			expected: false,
		},
		{
			name:     "case insensitive - UNAUTHORIZED",
			err:      fmt.Errorf("UNAUTHORIZED"),
			expected: true,
		},
		{
			name:     "case insensitive - Forbidden",
			err:      fmt.Errorf("Forbidden"),
			expected: true,
		},
		{
			name:     "substring match - contains unauthorized",
			err:      fmt.Errorf("Request failed: unauthorized access to resource"),
			expected: true,
		},
		{
			name:     "substring match - contains permission",
			err:      fmt.Errorf("User lacks permission to perform this action"),
			expected: true,
		},
		{
			name:     "no match - generic error",
			err:      fmt.Errorf("generic failure occurred"),
			expected: false,
		},
		{
			name:     "no match - network error",
			err:      fmt.Errorf("network connection timeout"),
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

	tests := []struct {
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
			ctx:  context.WithValue(context.Background(), "key", "value"),
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
	largeErr := fmt.Errorf("%s", largeMessage)

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
