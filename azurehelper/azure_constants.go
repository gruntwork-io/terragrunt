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

	// UUID generation constants
	uuidTimeMask32 = 0xFFFFFFFF     // Mask for first 32 bits of timestamp
	uuidTimeMask16 = 0xFFFF         // Mask for 16 bits of timestamp
	uuidTimeMask48 = 0xFFFFFFFFFFFF // Mask for 48 bits of microsecond timestamp
)
