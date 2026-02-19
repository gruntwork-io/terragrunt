package errorconfig

import "regexp"

// Exported aliases for internal symbols used by the external test package.
var (
	ExportExtractErrorMessage     = extractErrorMessage
	ExportMatchesAnyRegexpPattern = matchesAnyRegexpPattern
	ExportErrorCleanPattern       = errorCleanPattern
)

// ExportPattern wraps Pattern for external test access.
type ExportPattern = Pattern

// NewExportPattern creates a Pattern from a compiled regexp and negative flag.
func NewExportPattern(re *regexp.Regexp, negative bool) *Pattern {
	return &Pattern{Pattern: re, Negative: negative}
}
