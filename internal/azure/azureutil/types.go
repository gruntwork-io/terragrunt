// Package azureutil provides utility functions for Azure operations
package azureutil

import (
	"strings"
	"time"
)

// OperationType represents different Azure operations for telemetry
type OperationType string

const (
	// Operation types
	OperationBootstrap       OperationType = "bootstrap"
	OperationNeedsBootstrap  OperationType = "needs_bootstrap"
	OperationDelete          OperationType = "delete"
	OperationDeleteContainer OperationType = "delete_container"
	OperationDeleteAccount   OperationType = "delete_account"
	OperationMigrate         OperationType = "migrate"
	OperationContainerOp     OperationType = "container_operation"
	OperationStorageOp       OperationType = "storage_operation"
	OperationValidation      OperationType = "validation"
	OperationAuthentication  OperationType = "authentication"
)

// ErrorMetrics represents metrics collected for Azure errors
type ErrorMetrics struct {
	ErrorType      string
	Classification ErrorClass
	Operation      OperationType
	ResourceType   string
	ResourceName   string
	ErrorMessage   string
	IsRetryable    bool
	SubscriptionID string
	Location       string
	AuthMethod     string
	StatusCode     int
	RetryAttempts  int
	Duration       time.Duration
	Additional     map[string]interface{}
}

// ClassifyError determines the classification of an error based on its content
// Helper functions for string operations
func containsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if contains(s, substring) {
			return true
		}
	}
	return false
}

// Case-insensitive string contains
func contains(s, substring string) bool {
	s, substring = strings.ToLower(s), strings.ToLower(substring)
	return strings.Contains(s, substring)
}

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ""
	}

	errorString := err.Error()
	if errorString == "" {
		return ErrorClassUnknown
	}

	// Classify by content pattern matching
	// Order matters - more specific checks first
	switch {
	// Authentication/authorization errors (includes unauthorized, permission denied, access denied)
	case containsAny(errorString, "authentication", "auth", "unauthorized", "permission denied", "access denied", "invalid credentials", "token expired"):
		return ErrorClassAuthorization

	// Permission/RBAC errors (more specific permission handling)
	case containsAny(errorString, "rbac", "role assignment", "permission", "insufficient privileges"):
		return ErrorClassPermission

	// Request validation errors
	case containsAny(errorString, "config", "parameter", "argument", "flag", "missing required"):
		return ErrorClassInvalidRequest

	// Resource errors (container, storage, etc.) - takes precedence over not found for resource-related errors
	case containsAny(errorString, "container", "bucket", "storage", "account", "blob"):
		return ErrorClassResource

	// Network errors
	case containsAny(errorString, "network", "connection", "dial", "tcp", "http", "timeout", "timed out", "dns"):
		return ErrorClassNetworking

	// Not found errors
	case containsAny(errorString, "not found", "404", "does not exist"):
		return ErrorClassNotFound

	// Throttling/rate limiting errors
	case containsAny(errorString, "quota", "limit", "capacity", "exceeded", "throttl", "429", "too many requests", "rate limit"):
		return ErrorClassThrottling

	// System errors
	case containsAny(errorString, "system", "internal", "server error"):
		return ErrorClassSystem

	default:
		return ErrorClassUnknown
	}
}
