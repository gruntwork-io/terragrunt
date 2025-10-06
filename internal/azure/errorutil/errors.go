// Package errorutil provides centralized error handling utilities for Azure operations
package errorutil

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ErrorClass represents the classification of an Azure error
type ErrorClass string

// Predefined error classifications
const (
	ErrorClassUnknown        ErrorClass = "unknown"
	ErrorClassAuthentication ErrorClass = "authentication"
	ErrorClassPermission     ErrorClass = "permission"
	ErrorClassNotFound       ErrorClass = "not_found"
	ErrorClassNetworking     ErrorClass = "networking"
	ErrorClassInvalidRequest ErrorClass = "invalid_request"
	ErrorClassThrottling     ErrorClass = "throttling"
	ErrorClassTransient      ErrorClass = "transient"
	ErrorClassConfiguration  ErrorClass = "configuration"
	ErrorClassResource       ErrorClass = "resource"
)

// Resource types for error context
const (
	ResourceTypeBlob          = "blob"
	ResourceTypeContainer     = "container"
	ResourceTypeResourceGroup = "resource_group"
	ResourceTypeStorage       = "storage_account"
)

// AzureResponseError represents an Azure API error response with detailed information
type AzureResponseError struct {
	Message    string // Human-readable error message
	ErrorCode  string // Azure-specific error code
	StatusCode int    // HTTP status code from the Azure API response
}

// Error implements the error interface for AzureResponseError
func (e *AzureResponseError) Error() string {
	return fmt.Sprintf("Azure API error (StatusCode=%d, ErrorCode=%s): %s", e.StatusCode, e.ErrorCode, e.Message)
}

// AzureError represents a structured Azure error with additional context
//
//nolint:govet // fieldalignment: Field order mirrors error presentation and option application order.
type AzureError struct {
	Message        string
	ResourceType   string
	ResourceName   string
	RequestID      string
	ErrorCode      string
	Classification ErrorClass
	Cause          error
	StatusCode     int
}

// Error implements the error interface for AzureError
func (e *AzureError) Error() string {
	resourceInfo := ""
	if e.ResourceType != "" && e.ResourceName != "" {
		resourceInfo = fmt.Sprintf(" for %s '%s'", e.ResourceType, e.ResourceName)
	}

	requestInfo := ""
	if e.RequestID != "" {
		requestInfo = fmt.Sprintf(" (RequestID: %s)", e.RequestID)
	}

	return fmt.Sprintf("Azure error%s: %s%s", resourceInfo, e.Message, requestInfo)
}

// Unwrap returns the underlying cause of the error
func (e *AzureError) Unwrap() error {
	return e.Cause
}

// ErrorOption is a function type used to configure AzureError
type ErrorOption func(*AzureError)

// WithResourceType adds resource type information to the error
func WithResourceType(resourceType string) ErrorOption {
	return func(e *AzureError) {
		e.ResourceType = resourceType
	}
}

// WithResourceName adds resource name information to the error
func WithResourceName(resourceName string) ErrorOption {
	return func(e *AzureError) {
		e.ResourceName = resourceName
	}
}

// WithRequestID adds request ID information to the error
func WithRequestID(requestID string) ErrorOption {
	return func(e *AzureError) {
		e.RequestID = requestID
	}
}

// WithStatusCode adds HTTP status code information to the error
func WithStatusCode(statusCode int) ErrorOption {
	return func(e *AzureError) {
		e.StatusCode = statusCode
	}
}

// WithErrorCode adds Azure error code information to the error
func WithErrorCode(errorCode string) ErrorOption {
	return func(e *AzureError) {
		e.ErrorCode = errorCode
	}
}

// WithClassification adds error classification information to the error
func WithClassification(classification ErrorClass) ErrorOption {
	return func(e *AzureError) {
		e.Classification = classification
	}
}

// WithCause adds underlying cause information to the error
func WithCause(cause error) ErrorOption {
	return func(e *AzureError) {
		e.Cause = cause
	}
}

// NewError creates a new AzureError with the provided message and options
func NewError(msg string, opts ...ErrorOption) error {
	err := &AzureError{
		Message:        msg,
		Classification: ErrorClassUnknown,
	}
	for _, opt := range opts {
		opt(err)
	}

	return err
}

// NewPermissionError creates a new permission-related AzureError
func NewPermissionError(msg string, opts ...ErrorOption) error {
	err := &AzureError{
		Message:        msg,
		Classification: ErrorClassPermission,
	}
	for _, opt := range opts {
		opt(err)
	}

	return err
}

// IsPermissionError checks if the given error indicates a permission issue
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "permission") ||
		strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "insufficient privileges")
}

// IsNotFoundError checks if an error indicates a resource not found issue
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's already an AzureError with not_found classification
	var azErr *AzureError
	if errors.As(err, &azErr) && azErr.Classification == ErrorClassNotFound {
		return true
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist") ||
		strings.Contains(errStr, "404")
}

// ConvertAzureError converts an azcore.ResponseError to AzureResponseError
func ConvertAzureError(err error) *AzureResponseError {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		// Extract the error message from the error object
		message := respErr.Error()

		return &AzureResponseError{
			StatusCode: respErr.StatusCode,
			ErrorCode:  respErr.ErrorCode,
			Message:    message,
		}
	}

	return nil
}

// ClassifyError determines the error classification from an error
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassUnknown
	}

	// First try to use structured error analysis
	azureErr := ConvertAzureError(err)
	if azureErr != nil {
		// Use status codes for classification when available
		switch azureErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ErrorClassAuthentication
		case http.StatusNotFound:
			return ErrorClassNotFound
		case http.StatusTooManyRequests:
			return ErrorClassThrottling
		case http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return ErrorClassTransient
		}

		// Check specific Azure error codes
		switch azureErr.ErrorCode {
		case "StorageAccountNotFound", "ContainerNotFound", "BlobNotFound":
			return ErrorClassNotFound
		case "AuthorizationFailed", "Forbidden", "Unauthorized":
			return ErrorClassAuthentication
		case "InsufficientAccountPermissions", "AccessDenied":
			return ErrorClassPermission
		case "ThrottledRequest", "TooManyRequests":
			return ErrorClassThrottling
		case "InternalError", "ServiceUnavailable":
			return ErrorClassTransient
		}
	}

	// Fallback to string-based detection
	errStr := strings.ToLower(err.Error())

	switch {
	// Authentication/authorization errors (more comprehensive patterns)
	case strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "unauthenticated") ||
		strings.Contains(errStr, "invalid credentials") ||
		strings.Contains(errStr, "token expired") ||
		strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "access denied"):
		return ErrorClassAuthentication

	// Permission/RBAC errors (more specific permission handling)
	case strings.Contains(errStr, "rbac") ||
		strings.Contains(errStr, "role assignment") ||
		strings.Contains(errStr, "insufficient privileges"):
		return ErrorClassPermission

	// Resource errors (takes precedence over not found for resource-related errors)
	case strings.Contains(errStr, "container") ||
		strings.Contains(errStr, "storage account") ||
		strings.Contains(errStr, "blob"):
		return ErrorClassResource

	// Not found errors
	case strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "does not exist"):
		return ErrorClassNotFound

	// Network errors
	case strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "dial") ||
		strings.Contains(errStr, "tcp") ||
		strings.Contains(errStr, "http") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "dns"):
		return ErrorClassNetworking

	// Request validation errors (more specific patterns)
	case strings.Contains(errStr, "config") ||
		strings.Contains(errStr, "parameter") ||
		strings.Contains(errStr, "argument") ||
		strings.Contains(errStr, "flag") ||
		strings.Contains(errStr, "missing required"):
		return ErrorClassInvalidRequest

	// Throttling/rate limiting errors
	case strings.Contains(errStr, "throttled") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "capacity") ||
		strings.Contains(errStr, "exceeded") ||
		strings.Contains(errStr, "too many requests"):
		return ErrorClassThrottling

	// Transient/system errors
	case strings.Contains(errStr, "transient") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "retry") ||
		strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "system") ||
		strings.Contains(errStr, "internal"):
		return ErrorClassTransient

	// Configuration errors
	case strings.Contains(errStr, "missing") && (strings.Contains(errStr, "subscription") ||
		strings.Contains(errStr, "location") ||
		strings.Contains(errStr, "resource group")):
		return ErrorClassConfiguration

	default:
		return ErrorClassUnknown
	}
}

// WrapError wraps an error with additional Azure context
func WrapError(err error, message string, opts ...ErrorOption) error {
	if err == nil {
		return nil
	}

	// Start with options that come from the error itself
	options := []ErrorOption{
		WithCause(err),
		WithClassification(ClassifyError(err)),
	}

	// Add any additional options
	options = append(options, opts...)

	return NewError(fmt.Sprintf("%s: %v", message, err), options...)
}

// WrapBlobError wraps a blob-related error with context
func WrapBlobError(err error, container, key string) error {
	if err == nil {
		return nil
	}

	return WrapError(err, fmt.Sprintf("Error with blob '%s' in container '%s'", key, container),
		WithResourceType(ResourceTypeBlob),
		WithResourceName(fmt.Sprintf("%s/%s", container, key)))
}

// WrapContainerError wraps a container-related error with context
func WrapContainerError(err error, container string) error {
	if err == nil {
		return nil
	}

	return WrapError(err, fmt.Sprintf("Error with container '%s'", container),
		WithResourceType(ResourceTypeContainer),
		WithResourceName(container))
}

// WrapStorageAccountError wraps a storage account-related error with context
func WrapStorageAccountError(err error, accountName string) error {
	if err == nil {
		return nil
	}

	return WrapError(err, fmt.Sprintf("Error with storage account '%s'", accountName),
		WithResourceType(ResourceTypeStorage),
		WithResourceName(accountName))
}
