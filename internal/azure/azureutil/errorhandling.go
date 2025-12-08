// Package azureutil provides utility functions for Azure operations
package azureutil

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// TelemetryCollector defines the interface for telemetry collection
type TelemetryCollector interface {
	LogError(ctx context.Context, err error, operation OperationType, metrics ErrorMetrics)
	LogOperation(ctx context.Context, operation OperationType, duration time.Duration, attrs map[string]interface{})
}

// OperationHandler wraps operations with telemetry, error tracking, and retry logic.
// It collects metrics about operation duration, success/failure, and error types.
type OperationHandler struct {
	telemetry TelemetryCollector
	logger    log.Logger
}

// NewOperationHandler creates a new operation handler
func NewOperationHandler(telemetry TelemetryCollector, logger log.Logger) *OperationHandler {
	return &OperationHandler{
		telemetry: telemetry,
		logger:    logger,
	}
}

// WithErrorHandling wraps a function with error handling logic
// It takes an operation name, context, and a function to execute
// If the function returns an error, it will be logged with telemetry and wrapped with context
func (h *OperationHandler) WithErrorHandling(
	ctx context.Context,
	operation OperationType,
	resourceType string,
	resourceName string,
	fn func() error,
) error {
	startTime := time.Now()

	// Execute the function
	err := fn()

	// Calculate duration regardless of success/failure
	duration := time.Since(startTime)

	// If there was an error, log it with telemetry
	if err != nil {
		// Create error metrics
		metrics := ErrorMetrics{
			ErrorType:      h.determineErrorType(err),
			Classification: ClassifyError(err),
			Operation:      operation,
			ResourceType:   resourceType,
			ResourceName:   resourceName,
			ErrorMessage:   err.Error(),
			IsRetryable:    h.isRetryableError(err),
		}

		// Log the error with telemetry
		h.telemetry.LogError(ctx, err, operation, metrics)

		// Wrap the error with context
		return h.wrapError(err, operation, resourceType, resourceName)
	}

	// If successful, log the operation with telemetry
	attrs := map[string]interface{}{
		"status":      "success",
		"duration_ms": duration.Milliseconds(),
	}
	// Add resource information if provided
	if resourceType != "" {
		attrs["resource_type"] = resourceType
	}

	if resourceName != "" {
		attrs["resource_name"] = resourceName
	}

	h.telemetry.LogOperation(ctx, operation, duration, attrs)

	return nil
}

// WithRetryableErrorHandling is similar to WithErrorHandling but returns a boolean indicating if the error is retryable
func (h *OperationHandler) WithRetryableErrorHandling(
	ctx context.Context,
	operation OperationType,
	resourceType string,
	resourceName string,
	fn func() error,
) (error, bool) {
	err := h.WithErrorHandling(ctx, operation, resourceType, resourceName, fn)
	if err != nil {
		return err, h.isRetryableError(err)
	}

	return nil, false
}

// WithAuthErrorHandling wraps an authentication operation with proper error handling
// It uses the provided AuthConfig to provide context for authentication errors
// This is specifically designed for Azure authentication operations
func (h *OperationHandler) WithAuthErrorHandling(
	ctx context.Context,
	authConfig *azureauth.AuthConfig,
	fn func() error,
) error {
	startTime := time.Now()

	// Execute the function
	err := fn()

	// Calculate duration regardless of success/failure
	duration := time.Since(startTime)

	// If there was an error, log it with telemetry
	if err != nil {
		// Create error metrics with authentication-specific fields
		metrics := ErrorMetrics{
			ErrorType:      h.determineErrorType(err),
			Classification: ErrorClassAuthorization, // Always authorization for this function
			Operation:      OperationAuthentication,
			ResourceType:   "authentication",
			ResourceName:   string(authConfig.Method),
			ErrorMessage:   err.Error(),
			IsRetryable:    h.isRetryableError(err),
			AuthMethod:     string(authConfig.Method),
		}

		// Add subscription and tenant information if available
		if authConfig.SubscriptionID != "" {
			metrics.SubscriptionID = authConfig.SubscriptionID
		}

		// Log the error with telemetry
		h.telemetry.LogError(ctx, err, OperationAuthentication, metrics)

		// Wrap the error with authentication context
		return h.wrapAuthError(err, authConfig)
	}

	// If successful, log the operation with telemetry
	attrs := map[string]interface{}{
		"status":      "success",
		"duration_ms": duration.Milliseconds(),
		"auth_method": string(authConfig.Method),
	}

	// Add subscription ID if available
	if authConfig.SubscriptionID != "" {
		attrs["subscription_id"] = authConfig.SubscriptionID
	}

	h.telemetry.LogOperation(ctx, OperationAuthentication, duration, attrs)

	return nil
}

// Helper function to determine error type based on error content
func (h *OperationHandler) determineErrorType(err error) string {
	if err == nil {
		return ""
	}

	// Check for Azure SDK specific errors first using type assertions
	var azureErr *azcore.ResponseError
	if tgerrors.As(err, &azureErr) {
		// Map Azure service error codes to error types
		switch {
		case azureErr.StatusCode == 401 || azureErr.StatusCode == 403 ||
			strings.EqualFold(azureErr.ErrorCode, "AuthorizationFailed") ||
			strings.EqualFold(azureErr.ErrorCode, "AuthenticationFailed"):
			return "AzureAuthenticationError"

		case azureErr.StatusCode == 404 ||
			strings.EqualFold(azureErr.ErrorCode, "ResourceNotFound") ||
			strings.EqualFold(azureErr.ErrorCode, "ContainerNotFound") ||
			strings.EqualFold(azureErr.ErrorCode, "BlobNotFound"):
			return "AzureNotFoundError"

		case azureErr.StatusCode == 409 ||
			strings.EqualFold(azureErr.ErrorCode, "ResourceConflict") ||
			strings.EqualFold(azureErr.ErrorCode, "ContainerAlreadyExists") ||
			strings.EqualFold(azureErr.ErrorCode, "BlobAlreadyExists"):
			return "AzureConflictError"

		case azureErr.StatusCode == 400 || azureErr.StatusCode == 422 ||
			strings.Contains(azureErr.ErrorCode, "Invalid"):
			return "AzureValidationError"

		case azureErr.StatusCode == 408 ||
			strings.EqualFold(azureErr.ErrorCode, "OperationTimedOut"):
			return "AzureTimeoutError"

		case azureErr.StatusCode == 429 ||
			strings.EqualFold(azureErr.ErrorCode, "TooManyRequests") ||
			strings.Contains(azureErr.ErrorCode, "Throttl"):
			return "AzureThrottlingError"

		case azureErr.StatusCode >= 500 && azureErr.StatusCode < 600:
			return "AzureServerError"

		default:
			return "Azure:" + azureErr.ErrorCode
		}
	}

	// Extract the error string for further analysis
	errorString := err.Error()

	// Check for specific terragrunt error patterns in the error message
	switch {
	case strings.Contains(errorString, "Azure authentication failed"):
		return "AuthenticationError"

	case strings.Contains(errorString, "Container") && strings.Contains(errorString, "does not exist"):
		return "ContainerDoesNotExistError"

	case strings.Contains(errorString, "container name") &&
		(strings.Contains(errorString, "invalid") || strings.Contains(errorString, "must be")):
		return "ContainerValidationError"

	case strings.Contains(errorString, "error with storage account"):
		return "StorageAccountCreationError"

	case strings.Contains(errorString, "no valid authentication method found"):
		return "NoValidAuthMethodError"

	case containsAny(errorString, "authentication", "unauthorized", "unauthenticated", "permission denied"):
		return "AuthenticationError"

	case containsAny(errorString, "not found", "does not exist", "404", "no such file"):
		return "NotFoundError"

	case containsAny(errorString, "already exists", "conflict", "409", "duplicate"):
		return "ConflictError"

	case containsAny(errorString, "validate", "validation", "invalid", "malformed"):
		return "ValidationError"

	case containsAny(errorString, "timeout", "timed out", "deadline exceeded"):
		return "TimeoutError"

	case containsAny(errorString, "throttl", "rate limit", "429", "too many requests"):
		return "ThrottlingError"

	case containsAny(errorString, "connection", "network", "connect", "unreachable", "no route"):
		return "NetworkError"

	case containsAny(errorString, "permission", "access denied", "forbidden", "not authorized"):
		return "PermissionError"

	case containsAny(errorString, "quota", "limit exceeded", "insufficient", "capacity"):
		return "QuotaError"

	case containsAny(errorString, "configuration", "config", "settings"):
		return "ConfigurationError"
	}

	return "UnknownError"
}

// Helper function to determine if an error is retryable
func (h *OperationHandler) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Azure SDK specific errors first for more accurate retry decisions
	var azureErr *azcore.ResponseError
	if tgerrors.As(err, &azureErr) {
		// Common retryable status codes
		switch azureErr.StatusCode {
		case http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}

		// Retryable error codes
		switch {
		case strings.Contains(azureErr.ErrorCode, "Timeout"),
			strings.Contains(azureErr.ErrorCode, "Throttl"),
			strings.EqualFold(azureErr.ErrorCode, "OperationTimedOut"),
			strings.EqualFold(azureErr.ErrorCode, "ServerBusy"),
			strings.EqualFold(azureErr.ErrorCode, "ServiceUnavailable"),
			strings.EqualFold(azureErr.ErrorCode, "InternalServerError"),
			strings.EqualFold(azureErr.ErrorCode, "TooManyRequests"):
			return true
		}

		// Non-retryable status codes regardless of error message
		switch azureErr.StatusCode {
		case http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusConflict,
			http.StatusPreconditionFailed,
			http.StatusUnprocessableEntity:
			return false
		}
	}

	// Fall back to string pattern matching
	errorString := err.Error()

	// Check for explicit "retry" or "temporary" indicators
	if containsAny(errorString, "retry", "retryable", "temporary", "transient", "try again") {
		return true
	}

	// Common retryable error patterns
	return containsAny(errorString,
		"timeout", "timed out", "deadline exceeded",
		"throttl", "rate limit", "429", "too many requests",
		"500", "503", "service unavailable", "server busy",
		"connection reset", "EOF", "connection refused", "network",
		"intermittent")
}

// Helper function to wrap an error with context
func (h *OperationHandler) wrapError(err error, operation OperationType, resourceType, resourceName string) error {
	if err == nil {
		return nil
	}

	var message string

	switch {
	case resourceType != "" && resourceName != "":
		message = fmt.Sprintf("Azure %s operation failed for %s '%s'", operation, resourceType, resourceName)
	case resourceType != "":
		message = fmt.Sprintf("Azure %s operation failed for %s", operation, resourceType)
	default:
		message = fmt.Sprintf("Azure %s operation failed", operation)
	}

	return tgerrors.Errorf("%s: %w", message, err)
}

// Helper function to wrap an authentication error with context
func (h *OperationHandler) wrapAuthError(err error, authConfig *azureauth.AuthConfig) error {
	if err == nil {
		return nil
	}

	message := fmt.Sprintf("Azure authentication failed using %s", authConfig.Method)

	return tgerrors.Errorf("%s: %v", message, err)
}

// Helper methods are now defined in types.go

// Helper functions for string operations
func containsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(strings.ToLower(s), strings.ToLower(substring)) {
			return true
		}
	}

	return false
}
