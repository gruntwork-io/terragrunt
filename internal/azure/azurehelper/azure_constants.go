// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"net/http"
	"time"
)

const (
	// HTTP Status Codes
	httpStatusOK           = http.StatusOK
	httpStatusNotFound     = http.StatusNotFound
	httpStatusUnauthorized = http.StatusUnauthorized
	httpStatusForbidden    = http.StatusForbidden
	httpStatusConflict     = http.StatusConflict

	// Default timeout values
	defaultHTTPClientTimeout = 10 * time.Second
	defaultLocation          = "westeurope" // Default Azure region

	// Azure API versions
	defaultRoleAssignmentAPIVersion = "2022-04-01"

	// Storage Blob Data Owner role definition ID
	storageBlobDataOwnerRoleID = "b7e6dc6d-f1e8-4753-8033-0f276bb0955b"

	// Access tier constants
	AccessTierHot     = "Hot"
	AccessTierCool    = "Cool"
	AccessTierPremium = "Premium"

	// RBAC propagation retry configuration
	// Azure RBAC can take up to 5 minutes to propagate, so we use longer timeouts
	RbacRetryDelay         = 10 * time.Second // Wait between retry attempts
	RbacMaxRetries         = 30               // 30 retries * 10 seconds = 5 minutes max
	RbacRetryAttempts      = RbacMaxRetries   // Total attempts
	RbacPropagationTimeout = 5 * time.Minute  // Maximum time to wait for RBAC propagation
)
