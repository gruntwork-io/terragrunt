package azureutil

import (
	"fmt"
)

// ErrorClass represents the classification of an Azure error
type ErrorClass string

const (
	// Error classifications
	ErrorClassAuthorization  ErrorClass = "authorization"
	ErrorClassInvalidRequest ErrorClass = "invalid_request"
	ErrorClassNetworking     ErrorClass = "networking"
	ErrorClassNotFound       ErrorClass = "not_found"
	ErrorClassPermission     ErrorClass = "permission"
	ErrorClassResource       ErrorClass = "resource"
	ErrorClassSystem         ErrorClass = "system"
	ErrorClassThrottling     ErrorClass = "throttling"
	ErrorClassUnknown        ErrorClass = "unknown"
)

// AzureError represents a structured error from Azure operations
//
//lint:ignore fieldalignment -- Struct ordering matches public API expectations and doc layout.
type AzureError struct {
	Wrapped        error
	Message        string
	Suggestion     string
	ResourceType   string
	ResourceName   string
	Operation      string
	Classification ErrorClass
}

// Error implements the error interface
func (e *AzureError) Error() string {
	base := e.Message
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

// WithError adds an underlying error
func WithError(err error) ErrorOption {
	return func(e *AzureError) {
		e.Wrapped = err
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
func WithResourceType(resourceType string) ErrorOption {
	return func(e *AzureError) {
		e.ResourceType = resourceType
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

// newGenericError creates a new AzureError
func newGenericError(msg string, opts ...ErrorOption) *AzureError {
	err := &AzureError{
		Message:        msg,
		Classification: ErrorClassUnknown,
	}
	for _, opt := range opts {
		opt(err)
	}

	return err
}

// newPermissionError creates a new permission-related AzureError
func newPermissionError(msg string, opts ...ErrorOption) *AzureError {
	err := &AzureError{
		Message:        msg,
		Classification: ErrorClassPermission,
	}
	for _, opt := range opts {
		opt(err)
	}

	return err
}
