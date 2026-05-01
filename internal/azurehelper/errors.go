package azurehelper

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
)

// ErrorClass is a coarse classification of an Azure error, useful for retry
// decisions and user-facing messages.
type ErrorClass string

const (
	ErrorClassUnknown        ErrorClass = "unknown"
	ErrorClassAuthentication ErrorClass = "authentication"
	ErrorClassPermission     ErrorClass = "permission"
	ErrorClassNotFound       ErrorClass = "not_found"
	ErrorClassConflict       ErrorClass = "conflict"
	ErrorClassThrottling     ErrorClass = "throttling"
	ErrorClassTransient      ErrorClass = "transient"
	ErrorClassInvalidRequest ErrorClass = "invalid_request"
)

// ClassifyError returns a coarse ErrorClass for an Azure error based on the
// HTTP status code and Azure error code in an azcore.ResponseError. Returns
// ErrorClassUnknown for non-Azure errors or when no useful information is
// available.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassUnknown
	}

	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return ErrorClassUnknown
	}

	switch respErr.StatusCode {
	case http.StatusUnauthorized:
		return ErrorClassAuthentication
	case http.StatusForbidden:
		return ErrorClassPermission
	case http.StatusNotFound:
		return ErrorClassNotFound
	case http.StatusConflict:
		return ErrorClassConflict
	case http.StatusTooManyRequests:
		return ErrorClassThrottling
	case http.StatusBadRequest:
		return ErrorClassInvalidRequest
	}

	if respErr.StatusCode >= http.StatusInternalServerError {
		return ErrorClassTransient
	}

	return ErrorClassUnknown
}

// IsRetryable reports whether the error is one a caller may retry. This covers
// throttling (429), transient 5xx, and network-style errors that did not yield
// an azcore.ResponseError at all (treated as transient).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		// No HTTP response - likely a network or DNS error.
		return true
	}

	switch ClassifyError(err) {
	case ErrorClassThrottling, ErrorClassTransient:
		return true
	case ErrorClassUnknown,
		ErrorClassAuthentication,
		ErrorClassPermission,
		ErrorClassNotFound,
		ErrorClassConflict,
		ErrorClassInvalidRequest:
		return false
	}

	return false
}

// IsNotFound reports whether the error represents a 404 / ResourceNotFound
// from an Azure API.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		if respErr.StatusCode == http.StatusNotFound {
			return true
		}

		if strings.EqualFold(respErr.ErrorCode, "ResourceNotFound") ||
			strings.EqualFold(respErr.ErrorCode, "ContainerNotFound") ||
			strings.EqualFold(respErr.ErrorCode, "BlobNotFound") {
			return true
		}
	}

	return false
}

// WrapError wraps err with the Terragrunt internal errors package so the
// caller's stack trace is preserved, prefixing it with op for context.
// Returns nil if err is nil.
func WrapError(err error, op string) error {
	if err == nil {
		return nil
	}

	if op == "" {
		return tgerrors.New(err)
	}

	return tgerrors.Errorf("%s: %w", op, err)
}
