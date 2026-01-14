package util

import "strings"

// CleanString normalizes line endings across different platforms.
func CleanString(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
