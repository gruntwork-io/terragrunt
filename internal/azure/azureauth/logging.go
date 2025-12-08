// Package azureauth provides Azure authentication utilities
package azureauth

import "github.com/gruntwork-io/terragrunt/pkg/log"

// logDebug logs a debug message if the logger is not nil.
//
//nolint:unparam // args parameter is unused but kept for future flexibility
func logDebug(l log.Logger, msg string, _ ...interface{}) {
	if l != nil {
		l.Debug(msg)
	}
}
