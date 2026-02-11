// Package azurerm provides custom error types for Azure storage backend operations.
// These error types follow Terragrunt's error handling guidelines and support proper
// error wrapping and unwrapping for better debugging and error chain management.
package azurerm

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// MissingRequiredAzureRemoteStateConfig represents a missing required configuration parameter for Azure remote state.
// This error type is used when essential configuration fields are missing from the Azure backend configuration.
// The error contains the name of the missing configuration parameter to help with debugging.
//
// Common missing configuration parameters include:
// - "storage_account_name": The Azure Storage account name
// - "container_name": The blob container name for state storage
// - "key": The path/filename for the state file
// - "resource_group_name": The resource group containing the storage account
// - "subscription_id": The Azure subscription ID
//
// This error typically occurs when:
// - Required fields are not specified in the Terragrunt configuration
// - Environment variables are not set for required parameters
// - Configuration parsing fails to populate required fields
//
// Example error messages:
// - "missing required Azure remote state configuration storage_account_name"
// - "missing required Azure remote state configuration container_name"
// - "missing required Azure remote state configuration key"
type MissingRequiredAzureRemoteStateConfig string

// Error returns a string indicating that the Azure remote state configuration is missing a required parameter.
// The returned error message will include the name of the missing configuration parameter.
func (configName MissingRequiredAzureRemoteStateConfig) Error() string {
	return "missing required Azure remote state configuration " + string(configName)
}

// MaxRetriesWaitingForContainerExceeded represents an error when the maximum number of retries is exceeded
// while waiting for an Azure Storage container to become available.
type MaxRetriesWaitingForContainerExceeded string

// Error returns a string indicating that the maximum number of retries was exceeded
// while waiting for an Azure Storage container to become available.
func (err MaxRetriesWaitingForContainerExceeded) Error() string {
	return "Exceeded max retries waiting for Azure Storage container " + string(err)
}

// ContainerDoesNotExist represents an error when an Azure Storage container does not exist.
type ContainerDoesNotExist struct {
	Underlying    error
	ContainerName string
}

// Error returns a string indicating that an Azure Storage container does not exist,
// along with the underlying error details.
func (err ContainerDoesNotExist) Error() string {
	return fmt.Sprintf("Container %s does not exist. Underlying error: %v", err.ContainerName, err.Underlying)
}

// Unwrap returns the underlying error that caused the container to not exist.
func (err ContainerDoesNotExist) Unwrap() error {
	return err.Underlying
}

// MissingSubscriptionIDError represents an error when subscription_id is required but not provided.
type MissingSubscriptionIDError struct{}

// Error returns a string indicating that subscription_id is required for storage account operations.
func (err MissingSubscriptionIDError) Error() string {
	return "subscription_id is required for storage account creation"
}

// MissingLocationError represents an error when location is required but not provided.
type MissingLocationError struct{}

// Error returns a string indicating that location is required for storage account creation.
func (err MissingLocationError) Error() string {
	return "location is required for storage account creation"
}

// NoValidAuthMethodError represents an error when no valid authentication method is found.
type NoValidAuthMethodError struct{}

// Error returns a string indicating that no valid authentication method was found.
func (err NoValidAuthMethodError) Error() string {
	return "no valid authentication method found: Azure AD auth is recommended. Alternatively, provide one of: MSI, service principal credentials, or SAS token"
}

// StorageAccountCreationError wraps errors that occur during storage account creation or validation.
type StorageAccountCreationError struct {
	Underlying         error  // 16 bytes (interface)
	StorageAccountName string // 16 bytes (string)
}

// Error returns a string indicating that storage account creation or validation failed.
func (err StorageAccountCreationError) Error() string {
	return fmt.Sprintf("error with storage account %s: %v", err.StorageAccountName, err.Underlying)
}

// Unwrap returns the underlying error that caused the storage account operation to fail.
func (err StorageAccountCreationError) Unwrap() error {
	return err.Underlying
}

// ContainerCreationError is re-exported from azurehelper for convenience.
// Use azurehelper.ContainerCreationError directly when possible.
type ContainerCreationError = azurehelper.ContainerCreationError

// AuthenticationError wraps Azure authentication failures with context about the attempted auth method.
type AuthenticationError struct {
	Underlying error  // 16 bytes (interface)
	AuthMethod string // 16 bytes (string)
}

// Error returns a string indicating that Azure authentication failed.
func (err AuthenticationError) Error() string {
	return fmt.Sprintf("Azure authentication failed using %s: %v", err.AuthMethod, err.Underlying)
}

// Unwrap returns the underlying error that caused the authentication failure.
func (err AuthenticationError) Unwrap() error {
	return err.Underlying
}

// ContainerValidationError represents errors during container name validation.
type ContainerValidationError struct {
	ValidationIssue string // 16 bytes (string)
}

// Error returns a string indicating the container validation issue.
func (err ContainerValidationError) Error() string {
	return err.ValidationIssue
}

// MissingResourceGroupError represents an error when resource_group_name is required but not provided.
type MissingResourceGroupError struct{}

// Error returns a string indicating that resource_group_name is required.
func (err MissingResourceGroupError) Error() string {
	return "resource_group_name is required to delete a storage account"
}

// ServicePrincipalMissingSubscriptionIDError represents an error when subscription_id is required for service principal authentication.
type ServicePrincipalMissingSubscriptionIDError struct{}

// Error returns a string indicating that subscription_id is required when using service principal authentication.
func (err ServicePrincipalMissingSubscriptionIDError) Error() string {
	return "subscription_id is required when using service principal authentication"
}

// MultipleAuthMethodsSpecifiedError represents an error when multiple authentication methods are specified.
type MultipleAuthMethodsSpecifiedError struct{}

// Error returns a string indicating that multiple authentication methods cannot be specified simultaneously.
func (err MultipleAuthMethodsSpecifiedError) Error() string {
	return "cannot specify multiple authentication methods: choose one of storage account key, SAS token, service principal, Azure AD auth, or MSI"
}

// NonInteractiveDeleteRestrictionError represents an error when trying to delete a storage account in non-interactive mode.
type NonInteractiveDeleteRestrictionError struct {
	StorageAccountName string // 16 bytes (string)
}

// Error returns a string indicating that storage account deletion requires user confirmation in interactive mode.
func (err NonInteractiveDeleteRestrictionError) Error() string {
	return fmt.Sprintf("cannot delete storage account %s in non-interactive mode, user confirmation is required", err.StorageAccountName)
}

// IncompleteServicePrincipalConfigError represents an error when service principal configuration is incomplete.
// This error occurs when attempting to use Service Principal authentication but required fields are missing.
// The error provides information about which specific fields are missing to help with configuration debugging.
//
// Common missing fields include:
// - client_id: The Azure AD application (client) ID
// - client_secret: The client secret for the application
// - tenant_id: The Azure AD tenant ID
// - subscription_id: The Azure subscription ID
//
// This error is typically encountered when:
// - Environment variables are not set properly
// - Configuration files are missing required fields
// - Service Principal credentials are incomplete
//
// Example scenarios:
// - Missing AZURE_CLIENT_ID environment variable
// - Empty client_secret in configuration
// - Incorrect tenant_id format
type IncompleteServicePrincipalConfigError struct {
	// MissingFields contains the list of configuration fields that are missing or empty.
	// Field names correspond to the configuration keys (e.g., "client_id", "client_secret", "tenant_id").
	// This information helps users identify exactly which configuration values need to be provided.
	MissingFields []string
}

// Error returns a string indicating that service principal configuration is incomplete.
func (err IncompleteServicePrincipalConfigError) Error() string {
	return fmt.Sprintf("incomplete service principal configuration: missing required fields: %v", err.MissingFields)
}

// TransientAzureError represents a transient Azure API error that can be retried.
type TransientAzureError struct {
	Underlying error  // 16 bytes (interface)
	Operation  string // 16 bytes (string)
	StatusCode int    // 8 bytes (int)
}

// Error returns a string indicating that an Azure operation failed with a transient error.
func (err TransientAzureError) Error() string {
	return fmt.Sprintf("transient Azure error during %s (status: %d): %v", err.Operation, err.StatusCode, err.Underlying)
}

// Unwrap returns the underlying error that caused the transient failure.
func (err TransientAzureError) Unwrap() error {
	return err.Underlying
}

// IsRetryable returns true if this error represents a transient condition that should be retried.
func (err TransientAzureError) IsRetryable() bool {
	// Common transient HTTP status codes
	switch err.StatusCode {
	case http.StatusRequestTimeout, // 408 - Request Timeout
		http.StatusConflict,            // 409 - Conflict (often transient in Azure)
		http.StatusTooManyRequests,     // 429 - Too Many Requests
		http.StatusInternalServerError, // 500 - Internal Server Error
		http.StatusBadGateway,          // 502 - Bad Gateway
		http.StatusServiceUnavailable,  // 503 - Service Unavailable
		http.StatusGatewayTimeout:      // 504 - Gateway Timeout
		return true
	default:
		return false
	}
}

// MaxRetriesExceededError represents an error when the maximum number of retries is exceeded.
type MaxRetriesExceededError struct {
	Underlying   error         // 8 bytes (interface)
	Operation    string        // 16 bytes (string)
	MaxRetries   int           // 8 bytes (int)
	TotalElapsed time.Duration // 8 bytes (time.Duration)
}

// Error returns a string indicating that the maximum number of retries was exceeded.
func (err MaxRetriesExceededError) Error() string {
	return fmt.Sprintf("operation %s failed after %d retries (elapsed: %v): %v", err.Operation, err.MaxRetries, err.TotalElapsed, err.Underlying)
}

// Unwrap returns the underlying error that caused the final failure.
func (err MaxRetriesExceededError) Unwrap() error {
	return err.Underlying
}

// IsPermissionError checks if an error indicates a permission issue
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	// Check for common permission error patterns
	return strings.Contains(errStr, "permission") ||
		strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "insufficient") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "not authorized") ||
		strings.Contains(errStr, "authorization failed") ||
		strings.Contains(errStr, "role assignment") ||
		strings.Contains(errStr, "storage blob data owner")
}

// IsNotFoundError checks if an error indicates a resource not found issue
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	// Check for common not found patterns
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist") ||
		strings.Contains(errStr, "404")
}

// IsValidationError checks if an error indicates a validation issue
func IsValidationError(err error) bool {
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	// Check for common validation error patterns
	return strings.Contains(errStr, "validation") ||
		strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "must be") ||
		strings.Contains(errStr, "cannot")
}

// IsQuotaError checks if an error indicates a quota or limit issue
func IsQuotaError(err error) bool {
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := strings.ToLower(err.Error())

	// Check for common quota/limit error patterns
	return strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "exceeded")
}

// Helper functions for common error patterns to reduce code duplication

// WrapStorageAccountError wraps an error as a StorageAccountCreationError with context
func WrapStorageAccountError(err error, storageAccountName string) error {
	if err == nil {
		return nil
	}

	return errors.New(StorageAccountCreationError{
		Underlying:         err,
		StorageAccountName: storageAccountName,
	})
}

// WrapContainerError wraps an error as a ContainerCreationError with context
func WrapContainerError(err error, containerName string) error {
	if err == nil {
		return nil
	}

	return errors.New(azurehelper.NewContainerCreationError(err, containerName))
}

// WrapAuthenticationError wraps an error as an AuthenticationError with context
func WrapAuthenticationError(err error, authMethod string) error {
	if err == nil {
		return nil
	}

	return errors.New(AuthenticationError{
		Underlying: err,
		AuthMethod: authMethod,
	})
}

// WrapContainerDoesNotExistError wraps an error as a ContainerDoesNotExist error
func WrapContainerDoesNotExistError(err error, containerName string) error {
	if err == nil {
		return nil
	}

	return errors.New(ContainerDoesNotExist{
		Underlying:    err,
		ContainerName: containerName,
	})
}

// WrapConfigMissingError creates a MissingRequiredAzureRemoteStateConfig error
func WrapConfigMissingError(configName string) error {
	return errors.New(MissingRequiredAzureRemoteStateConfig(configName))
}

// WrapContainerValidationError creates a ContainerValidationError with the given validation issue
func WrapContainerValidationError(validationIssue string) error {
	return errors.New(ContainerValidationError{
		ValidationIssue: validationIssue,
	})
}

// WrapNonInteractiveDeleteError creates a NonInteractiveDeleteRestrictionError
func WrapNonInteractiveDeleteError(storageAccountName string) error {
	return errors.New(NonInteractiveDeleteRestrictionError{
		StorageAccountName: storageAccountName,
	})
}

// WrapIncompleteServicePrincipalError creates an IncompleteServicePrincipalConfigError
func WrapIncompleteServicePrincipalError(missingFields []string) error {
	return errors.New(IncompleteServicePrincipalConfigError{
		MissingFields: missingFields,
	})
}

// WrapAzureClientError wraps errors from Azure client creation with appropriate context
func WrapAzureClientError(err error, clientType, resourceName string) error {
	if err == nil {
		return nil
	}

	// Different client types require different error wrapping
	switch clientType {
	case "BlobService", "StorageAccount":
		return WrapStorageAccountError(err, resourceName)
	case "Container":
		return WrapContainerError(err, resourceName)
	default:
		return errors.Errorf("Azure %s client error for resource %s: %w", clientType, resourceName, err)
	}
}

// WrapTransientAzureError wraps an error as a TransientAzureError with context
func WrapTransientAzureError(err error, operation string, statusCode int) error {
	if err == nil {
		return nil
	}

	return errors.New(TransientAzureError{
		Underlying: err,
		Operation:  operation,
		StatusCode: statusCode,
	})
}

// WrapMaxRetriesExceededError wraps an error as a MaxRetriesExceededError with context
func WrapMaxRetriesExceededError(err error, operation string, maxRetries int, totalElapsed time.Duration) error {
	if err == nil {
		return nil
	}

	return errors.New(MaxRetriesExceededError{
		Underlying:   err,
		Operation:    operation,
		MaxRetries:   maxRetries,
		TotalElapsed: totalElapsed,
	})
}

// Helper functions for common singleton errors (errors without parameters)

// NewMissingSubscriptionIDError creates a new MissingSubscriptionIDError
func NewMissingSubscriptionIDError() error {
	return errors.New(MissingSubscriptionIDError{})
}

// NewMissingLocationError creates a new MissingLocationError
func NewMissingLocationError() error {
	return errors.New(MissingLocationError{})
}

// NewNoValidAuthMethodError creates a new NoValidAuthMethodError
func NewNoValidAuthMethodError() error {
	return errors.New(NoValidAuthMethodError{})
}

// NewMissingResourceGroupError creates a new MissingResourceGroupError
func NewMissingResourceGroupError() error {
	return errors.New(MissingResourceGroupError{})
}

// NewServicePrincipalMissingSubscriptionIDError creates a new ServicePrincipalMissingSubscriptionIDError
func NewServicePrincipalMissingSubscriptionIDError() error {
	return errors.New(ServicePrincipalMissingSubscriptionIDError{})
}

// NewMultipleAuthMethodsSpecifiedError creates a new MultipleAuthMethodsSpecifiedError
func NewMultipleAuthMethodsSpecifiedError() error {
	return errors.New(MultipleAuthMethodsSpecifiedError{})
}

// WrapError provides a unified way to wrap Azure errors with context
func WrapError(err error, operation string, resourceType string, resourceName string) error {
	if err == nil {
		return nil
	}

	// First classify the error
	switch {
	case IsPermissionError(err):
		return WrapAuthenticationError(err, "permission denied")
	case IsNotFoundError(err):
		switch resourceType {
		case "StorageAccount":
			return WrapStorageAccountError(err, resourceName)
		case "Container":
			return WrapContainerDoesNotExistError(err, resourceName)
		default:
			return errors.Errorf("%s %s not found: %w", resourceType, resourceName, err)
		}
	case IsValidationError(err):
		switch resourceType {
		case "Container":
			return WrapContainerValidationError(err.Error())
		default:
			return errors.Errorf("%s validation failed: %w", resourceType, err)
		}
	case IsQuotaError(err):
		return errors.Errorf("%s quota exceeded for %s: %w", resourceType, resourceName, err)
	case IsRetryableError(err):
		// Extract status code if available
		statusCode := extractStatusCode(err.Error())
		return WrapTransientAzureError(err, operation, statusCode)
	default:
		// For unclassified errors, still provide context
		return errors.Errorf("%s operation failed for %s %s: %w", operation, resourceType, resourceName, err)
	}
}

// Error checking helpers

// GetErrorContext extracts classification and operation info from an error
func GetErrorContext(err error) (ErrorClassification, OperationType, string) {
	if err == nil {
		return "", "", ""
	}

	// Get classification first
	classification := ClassifyError(err)

	// Determine operation type and details
	var (
		operationType OperationType
		details       string
	)

	switch {
	case errors.As(err, &StorageAccountCreationError{}):
		operationType = OperationStorageOp
		details = "storage account operation"
	case errors.As(err, &ContainerCreationError{}):
		operationType = OperationContainerOp
		details = "container operation"
	case errors.As(err, &AuthenticationError{}):
		operationType = OperationAuthentication
		details = "authentication"
	case errors.As(err, &ContainerValidationError{}):
		operationType = OperationValidation
		details = "validation"
	default:
		operationType = "" // Let caller decide default
		details = "unknown operation"
	}

	return classification, operationType, details
}

// WrapErrorWithTelemetry wraps an error and logs telemetry data
func WrapErrorWithTelemetry(ctx context.Context, err error, tel *AzureTelemetryCollector, operation OperationType, resourceType string, resourceName string) error {
	if err == nil {
		return nil
	}

	// Get error context
	classification, _, details := GetErrorContext(err)

	// Log error with telemetry
	tel.LogError(ctx, err, operation, AzureErrorMetrics{
		Classification: classification,
		Operation:      operation,
		ResourceType:   resourceType,
		ResourceName:   resourceName,
		ErrorMessage:   details,
		Additional: map[string]interface{}{
			"error_type": reflect.TypeOf(err).String(),
		},
	})

	// Wrap the error with context
	return WrapError(err, string(operation), resourceType, resourceName)
}
