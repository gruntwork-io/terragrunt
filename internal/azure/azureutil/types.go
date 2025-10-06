// Package azureutil provides utility functions for Azure operations
package azureutil

import (
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/errorutil"
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
//
//nolint:govet // fieldalignment: Layout chosen for semantic grouping and JSON stability.
type ErrorMetrics struct {
	ErrorType      string
	ResourceType   string
	ResourceName   string
	ErrorMessage   string
	SubscriptionID string
	Location       string
	AuthMethod     string
	Classification ErrorClass
	Operation      OperationType
	Duration       time.Duration
	StatusCode     int
	RetryAttempts  int
	Additional     map[string]interface{}
	IsRetryable    bool
}

// ClassifyError determines the classification of an error based on its content

func ClassifyError(err error) ErrorClass {
	// Convert from errorutil.ErrorClass to local ErrorClass enum
	switch errorutil.ClassifyError(err) {
	case errorutil.ErrorClassAuthentication:
		return ErrorClassAuthorization
	case errorutil.ErrorClassPermission:
		return ErrorClassPermission
	case errorutil.ErrorClassInvalidRequest:
		return ErrorClassInvalidRequest
	case errorutil.ErrorClassConfiguration:
		return ErrorClassInvalidRequest // Map configuration errors to invalid request
	case errorutil.ErrorClassNetworking:
		return ErrorClassNetworking
	case errorutil.ErrorClassNotFound:
		return ErrorClassNotFound
	case errorutil.ErrorClassResource:
		return ErrorClassResource
	case errorutil.ErrorClassThrottling:
		return ErrorClassThrottling
	case errorutil.ErrorClassTransient:
		return ErrorClassSystem // Map transient errors to system
	case errorutil.ErrorClassUnknown:
		return ErrorClassUnknown
	default:
		return ErrorClassUnknown
	}
}
