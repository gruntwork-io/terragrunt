// Package azurehelper provides Azure-specific helper functions
package azurehelper

import "github.com/gruntwork-io/terragrunt/pkg/log"

// logDebug logs a debug message if the logger is not nil.
func logDebug(l log.Logger, msg string, args ...interface{}) {
	if l != nil {
		l.Debugf(msg, args...)
	}
}

// logInfo logs an info message if the logger is not nil.
func logInfo(l log.Logger, msg string, args ...interface{}) {
	if l != nil {
		l.Infof(msg, args...)
	}
}

// logWarn logs a warning message if the logger is not nil.
func logWarn(l log.Logger, msg string, args ...interface{}) {
	if l != nil {
		l.Warnf(msg, args...)
	}
}
