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
	RbacRetryDelay    = 3 * time.Second
	RbacMaxRetries    = 5
	RbacRetryAttempts = RbacMaxRetries + 1 // Total attempts including first try
)
