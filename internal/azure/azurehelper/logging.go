// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

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

const (
	redactedText          = "***REDACTED***"
	accountKeyPrefix      = "AccountKey="
	sharedAccessKeyPrefix = "SharedAccessKey="
)

// RedactSensitiveValue redacts sensitive values from environment variable values for safe logging.
// It fully redacts keys containing sensitive keywords, and partially redacts connection strings
// by preserving non-sensitive parts like AccountName while hiding AccountKey and SharedAccessKey.
func RedactSensitiveValue(key, value string) string {
	if isSensitiveKey(key) {
		return redactedText
	}

	if isConnectionString(value) {
		return redactConnectionString(value)
	}

	// Return as-is if not sensitive
	return value
}

// isSensitiveKey checks if an environment variable key should be fully redacted.
func isSensitiveKey(key string) bool {
	return key == "AZURE_CLIENT_SECRET" ||
		key == "AZURE_CLIENT_CERTIFICATE_PASSWORD" ||
		strings.Contains(key, "_KEY") ||
		strings.Contains(key, "PASSWORD") ||
		strings.Contains(key, "SECRET")
}

// isConnectionString checks if a value looks like an Azure connection string with sensitive data.
func isConnectionString(value string) bool {
	valueLower := strings.ToLower(value)
	return strings.Contains(value, ";") &&
		(strings.Contains(valueLower, strings.ToLower(accountKeyPrefix)) ||
			strings.Contains(valueLower, strings.ToLower(sharedAccessKeyPrefix)))
}

// redactConnectionString redacts sensitive parts of a connection string while preserving structure.
func redactConnectionString(value string) string {
	parts := strings.Split(value, ";")

	var safeParts []string

	for _, part := range parts {
		partLower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(partLower, strings.ToLower(accountKeyPrefix)):
			// Preserve original key casing, only redact the value
			if idx := strings.Index(part, "="); idx != -1 {
				safeParts = append(safeParts, part[:idx+1]+redactedText)
			} else {
				safeParts = append(safeParts, accountKeyPrefix+redactedText)
			}
		case strings.HasPrefix(partLower, strings.ToLower(sharedAccessKeyPrefix)):
			// Preserve original key casing, only redact the value
			if idx := strings.Index(part, "="); idx != -1 {
				safeParts = append(safeParts, part[:idx+1]+redactedText)
			} else {
				safeParts = append(safeParts, sharedAccessKeyPrefix+redactedText)
			}
		default:
			safeParts = append(safeParts, part)
		}
	}

	return strings.Join(safeParts, ";")
}
