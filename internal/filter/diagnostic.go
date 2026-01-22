package filter

import (
	"fmt"
	"strings"
)

// ANSI escape codes for colored output.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiItalic = "\033[3m"
	ansiRed    = "\033[31m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

// FormatDiagnostic produces a Rust-style error message from a ParseError.
func FormatDiagnostic(err *ParseError, filterIndex int, useColor bool) string {
	var sb strings.Builder

	// Line 1: Error header
	if useColor {
		fmt.Fprintf(&sb, "%s%serror:%s %s\n", ansiBold, ansiRed, ansiReset, err.Message)
	} else {
		fmt.Fprintf(&sb, "error: %s\n", err.Message)
	}

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

	// Line 5: Underline at error position
	indent := "     " // 5 spaces for alignment
	spaces := strings.Repeat(" ", err.Position)
	underlineLen := max(1, err.TokenLength)
	underline := strings.Repeat("^", underlineLen)
	detail := " " + err.Message

	if useColor {
		fmt.Fprintf(&sb, "%s%s%s%s%s%s%s\n", indent, spaces, ansiBold, ansiRed, underline, ansiReset, detail)
	} else {
		fmt.Fprintf(&sb, "%s%s%s%s\n", indent, spaces, underline, detail)
	}

	// Line 6: Blank line before hints
	sb.WriteString("\n")

	// Line 7+: Hints
	hints := GetHints(err.ErrorCode, err.TokenLiteral, err.Query, err.Position)
	for _, hint := range hints {
		if useColor {
			fmt.Fprintf(&sb, "  %s%shint:%s %s%s%s\n", ansiBold, ansiCyan, ansiReset, ansiItalic, hint, ansiReset)
		} else {
			fmt.Fprintf(&sb, "  hint: %s\n", hint)
		}
	}

	return sb.String()
}
