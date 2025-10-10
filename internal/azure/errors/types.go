// Package errors provides error types and handling for Azure operations
package errors

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/azure/errorutil"
	terragruntErrors "github.com/gruntwork-io/terragrunt/internal/errors"
)

// FormatWithStackTrace formats a string with arguments and ensures stack traces are preserved
// This is a helper function to replace fmt.Sprintf while ensuring proper error handling
func FormatWithStackTrace(format string, args ...interface{}) string {
	for i, arg := range args {
		if err, ok := arg.(error); ok && err != nil {
			// Replace error arguments with stack-traced versions
			args[i] = terragruntErrors.New(err)
		}
	}

	return fmt.Sprintf(format, args...)
}

// ErrorClass represents the classification of an Azure error
type ErrorClass string

const (
	// Error classifications
	ErrorClassAuthentication ErrorClass = "authentication"
	ErrorClassAuthorization  ErrorClass = "authorization"
	ErrorClassConfiguration  ErrorClass = "configuration"
	ErrorClassInvalidRequest ErrorClass = "invalid_request"
	ErrorClassNetworking     ErrorClass = "networking"
	ErrorClassNotFound       ErrorClass = "not_found"
	ErrorClassPermission     ErrorClass = "permission"
	ErrorClassResource       ErrorClass = "resource"
	ErrorClassSystem         ErrorClass = "system"
	ErrorClassThrottling     ErrorClass = "throttling"
	ErrorClassTransient      ErrorClass = "transient"
	ErrorClassUnknown        ErrorClass = "unknown"
)

// ResourceType represents the type of Azure resource
type ResourceType string

const (
	// Resource types
	ResourceTypeBlob          ResourceType = "blob"
	ResourceTypeContainer     ResourceType = "container"
	ResourceTypeResourceGroup ResourceType = "resource_group"
	ResourceTypeStorage       ResourceType = "storage_account"
)

// AzureError represents a structured error from Azure operations
type AzureError struct {
	Message        string
	Wrapped        error
	Suggestion     string
	Classification ErrorClass
	ResourceType   ResourceType
	ResourceName   string
	Operation      string
}

// Error implements the error interface
func (e *AzureError) Error() string {
	base := e.Message

	// Add operation context if available
	if e.Operation != "" {
		base = fmt.Sprintf("%s (operation: %s)", base, e.Operation)
	}

	// Add resource context if available
	if e.ResourceType != "" || e.ResourceName != "" {
		switch {
		case e.ResourceType != "" && e.ResourceName != "":
			base = fmt.Sprintf("%s [%s: %s]", base, e.ResourceType, e.ResourceName)
		case e.ResourceType != "":
			base = fmt.Sprintf("%s [%s]", base, e.ResourceType)
		case e.ResourceName != "":
			base = fmt.Sprintf("%s [%s]", base, e.ResourceName)
		}
	}

	if e.Wrapped != nil {
		base = fmt.Sprintf("%s: %v", base, e.Wrapped)
	}

	if e.Suggestion != "" {
		base = fmt.Sprintf("%s\nSuggestion: %s", base, e.Suggestion)
	}

	return base
}

// Unwrap returns the underlying error
func (e *AzureError) Unwrap() error {
	return e.Wrapped
}

// ErrorOption is a functional option for configuring an AzureError
type ErrorOption func(*AzureError)

// WithError adds an underlying error with stack trace
func WithError(err error) ErrorOption {
	return func(e *AzureError) {
		if err != nil {
			e.Wrapped = terragruntErrors.New(err)
		} else {
			e.Wrapped = nil
		}
	}
}

// WithSuggestion adds a suggestion to the error
func WithSuggestion(suggestion string) ErrorOption {
	return func(e *AzureError) {
		e.Suggestion = suggestion
	}
}

// WithClassification sets the error classification
func WithClassification(class ErrorClass) ErrorOption {
	return func(e *AzureError) {
		e.Classification = class
	}
}

// WithResourceType sets the resource type
func WithResourceType(resType ResourceType) ErrorOption {
	return func(e *AzureError) {
		e.ResourceType = resType
	}
}

// WithResourceName sets the resource name
func WithResourceName(name string) ErrorOption {
	return func(e *AzureError) {
		e.ResourceName = name
	}
}

// WithOperation sets the operation that caused the error
func WithOperation(op string) ErrorOption {
	return func(e *AzureError) {
		e.Operation = op
	}
}

// NewGenericError creates a new AzureError
func NewGenericError(msg string, opts ...ErrorOption) error {
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
	// Using the centralized errorutil implementation
	return errorutil.IsPermissionError(err)
}

// ClassifyError determines the error classification from an error
func ClassifyError(err error) ErrorClass {
	// Convert from errorutil.ErrorClass to local ErrorClass type
	return ErrorClass(errorutil.ClassifyError(err))
}
