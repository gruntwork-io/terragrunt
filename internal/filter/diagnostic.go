package filter

import (
	"fmt"
	"strings"
)

// ANSI escape codes for colored output.
const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiRed   = "\033[31m"
	ansiBlue  = "\033[34m"
	ansiCyan  = "\033[36m"
)

// FormatDiagnostic produces an error message from a ParseError.
//
// These diagnostics are formatted like so:
//
// ```
// Filter parsing error: Missing Git reference
//
//	--> --filter '[main...]'
//
//	    [main...]
//	            ^ Expected second Git reference after '...'
//
//	 hint: Git filters with '...' require a reference on each side. e.g. '[main...HEAD]'
//
// ```
func FormatDiagnostic(err *ParseError, filterIndex int, useColor bool) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Filter parsing error: %s\n", err.Title)

	var arrow string
	if useColor {
		arrow = fmt.Sprintf("%s%s --> %s", ansiBold, ansiBlue, ansiReset)
	} else {
		arrow = " --> "
	}

	if filterIndex > 0 {
		fmt.Fprintf(&sb, "%s--filter[%d] '%s'\n", arrow, filterIndex, err.Query)
	} else {
		fmt.Fprintf(&sb, "%s--filter '%s'\n", arrow, err.Query)
	}

	sb.WriteString("\n")

	fmt.Fprintf(&sb, "     %s\n", err.Query)

	indent := "     "
	spaces := strings.Repeat(" ", err.ErrorPosition)
	caret := "^"
	detail := " " + err.Message

	if useColor {
		fmt.Fprintf(&sb, "%s%s%s%s%s%s%s\n", indent, spaces, ansiBold, ansiRed, caret, ansiReset, detail)
	} else {
		fmt.Fprintf(&sb, "%s%s%s%s\n", indent, spaces, caret, detail)
	}

	hint := GetHint(err.ErrorCode, err.TokenLiteral, err.Query, err.Position)
	if hint != "" {
		sb.WriteString("\n")

		if useColor {
			fmt.Fprintf(&sb, "  %s%shint:%s %s\n", ansiBold, ansiCyan, ansiReset, hint)
		} else {
			fmt.Fprintf(&sb, "  hint: %s\n", hint)
		}
	}

	return sb.String()
}
