// Package azureauth provides Azure authentication utilities
package azureauth

import "github.com/gruntwork-io/terragrunt/pkg/log"

// logDebug logs a debug message if the logger is not nil.
func logDebug(l log.Logger, msg string, args ...interface{}) {
	if l != nil {
		l.Debugf(msg, args...)
	}
}
