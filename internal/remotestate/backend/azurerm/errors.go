// Package azurerm provides custom error types for Azure storage backend operations.
// These error types follow Terragrunt's error handling guidelines and support proper
// error wrapping and unwrapping for better debugging and error chain management.
package azurerm

import (
	"fmt"
	"time"

	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
)

// MissingRequiredAzureRemoteStateConfig represents a missing required configuration parameter for Azure remote state.
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
	Underlying         error  // 8 bytes (interface)
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

// ContainerCreationError wraps errors that occur during Azure container operations.
type ContainerCreationError struct {
	Underlying    error  // 8 bytes (interface)
	ContainerName string // 16 bytes (string)
}

// Error returns a string indicating that container operation failed.
func (err ContainerCreationError) Error() string {
	return fmt.Sprintf("error with container %s: %v", err.ContainerName, err.Underlying)
}

// Unwrap returns the underlying error that caused the container operation to fail.
func (err ContainerCreationError) Unwrap() error {
	return err.Underlying
}

// AuthenticationError wraps Azure authentication failures with context about the attempted auth method.
type AuthenticationError struct {
	Underlying error  // 8 bytes (interface)
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
type IncompleteServicePrincipalConfigError struct {
	MissingFields []string // 24 bytes (slice)
}

// Error returns a string indicating that service principal configuration is incomplete.
func (err IncompleteServicePrincipalConfigError) Error() string {
	return fmt.Sprintf("incomplete service principal configuration: missing required fields: %v", err.MissingFields)
}

// TransientAzureError represents a transient Azure API error that can be retried.
type TransientAzureError struct {
	Underlying error  // 8 bytes (interface)
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
	case 429, // Too Many Requests
		500, // Internal Server Error
		502, // Bad Gateway
		503, // Service Unavailable
		504: // Gateway Timeout
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

// Helper functions for common error patterns to reduce code duplication

// WrapStorageAccountError wraps an error as a StorageAccountCreationError with context
func WrapStorageAccountError(err error, storageAccountName string) error {
	if err == nil {
		return nil
	}
	return tgerrors.New(StorageAccountCreationError{
		Underlying:         err,
		StorageAccountName: storageAccountName,
	})
}

// WrapContainerError wraps an error as a ContainerCreationError with context
func WrapContainerError(err error, containerName string) error {
	if err == nil {
		return nil
	}
	return tgerrors.New(ContainerCreationError{
		Underlying:    err,
		ContainerName: containerName,
	})
}

// WrapAuthenticationError wraps an error as an AuthenticationError with context
func WrapAuthenticationError(err error, authMethod string) error {
	if err == nil {
		return nil
	}
	return tgerrors.New(AuthenticationError{
		Underlying: err,
		AuthMethod: authMethod,
	})
}

// WrapContainerDoesNotExistError wraps an error as a ContainerDoesNotExist error
func WrapContainerDoesNotExistError(err error, containerName string) error {
	if err == nil {
		return nil
	}
	return tgerrors.New(ContainerDoesNotExist{
		Underlying:    err,
		ContainerName: containerName,
	})
}

// WrapConfigMissingError creates a MissingRequiredAzureRemoteStateConfig error
func WrapConfigMissingError(configName string) error {
	return tgerrors.New(MissingRequiredAzureRemoteStateConfig(configName))
}

// WrapContainerValidationError creates a ContainerValidationError with the given validation issue
func WrapContainerValidationError(validationIssue string) error {
	return tgerrors.New(ContainerValidationError{
		ValidationIssue: validationIssue,
	})
}

// WrapNonInteractiveDeleteError creates a NonInteractiveDeleteRestrictionError
func WrapNonInteractiveDeleteError(storageAccountName string) error {
	return tgerrors.New(NonInteractiveDeleteRestrictionError{
		StorageAccountName: storageAccountName,
	})
}

// WrapIncompleteServicePrincipalError creates an IncompleteServicePrincipalConfigError
func WrapIncompleteServicePrincipalError(missingFields []string) error {
	return tgerrors.New(IncompleteServicePrincipalConfigError{
		MissingFields: missingFields,
	})
}

// Helper functions for common singleton errors (errors without parameters)

// NewMissingSubscriptionIDError creates a new MissingSubscriptionIDError
func NewMissingSubscriptionIDError() error {
	return tgerrors.New(MissingSubscriptionIDError{})
}

// NewMissingLocationError creates a new MissingLocationError
func NewMissingLocationError() error {
	return tgerrors.New(MissingLocationError{})
}

// NewNoValidAuthMethodError creates a new NoValidAuthMethodError
func NewNoValidAuthMethodError() error {
	return tgerrors.New(NoValidAuthMethodError{})
}

// NewMissingResourceGroupError creates a new MissingResourceGroupError
func NewMissingResourceGroupError() error {
	return tgerrors.New(MissingResourceGroupError{})
}

// NewServicePrincipalMissingSubscriptionIDError creates a new ServicePrincipalMissingSubscriptionIDError
func NewServicePrincipalMissingSubscriptionIDError() error {
	return tgerrors.New(ServicePrincipalMissingSubscriptionIDError{})
}

// NewMultipleAuthMethodsSpecifiedError creates a new MultipleAuthMethodsSpecifiedError
func NewMultipleAuthMethodsSpecifiedError() error {
	return tgerrors.New(MultipleAuthMethodsSpecifiedError{})
}

// Helper functions for Azure-specific error patterns

// WrapAzureAuthError wraps Azure authentication errors with context about auth method
func WrapAzureAuthError(err error, authMethod string) error {
	if err == nil {
		return nil
	}
	return WrapAuthenticationError(err, authMethod)
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
		return err
	}
}
