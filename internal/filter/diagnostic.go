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

// FormatDiagnostic produces a Rust-style error message from a ParseError.
func FormatDiagnostic(err *ParseError, filterIndex int, useColor bool) string {
	var sb strings.Builder

	// Line 1: Error header with high-level title
	fmt.Fprintf(&sb, "Filter parsing error: %s\n", err.Title)

	// Line 2: Location arrow
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

	// Line 3: Blank line
	sb.WriteString("\n")

	// Line 4: The query with indentation
	fmt.Fprintf(&sb, "     %s\n", err.Query)

	// Line 5: Caret at ErrorPosition (may differ from Position for unclosed brackets)
	indent := "     " // 5 spaces for alignment
	spaces := strings.Repeat(" ", err.ErrorPosition)
	caret := "^"
	detail := " " + err.Message

	if useColor {
		fmt.Fprintf(&sb, "%s%s%s%s%s%s%s\n", indent, spaces, ansiBold, ansiRed, caret, ansiReset, detail)
	} else {
		fmt.Fprintf(&sb, "%s%s%s%s\n", indent, spaces, caret, detail)
	}

	// Line 6-7: Blank line and hint (only if hint exists)
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
