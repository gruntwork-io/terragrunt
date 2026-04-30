// Package azureutil provides utility functions for Azure operations
package azureutil

import (
	"fmt"
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
	Classification errorutil.ErrorClass
	Operation      OperationType
	Duration       time.Duration
	StatusCode     int
	RetryAttempts  int
	Additional     map[string]interface{}
	IsRetryable    bool
}

// Error implements the error interface for ErrorMetrics.
func (e *ErrorMetrics) Error() string {
	if e.ErrorMessage != "" {
		return e.ErrorMessage
	}

	return fmt.Sprintf("%s error: %s (status: %d)", e.ErrorType, e.Classification, e.StatusCode)
}
