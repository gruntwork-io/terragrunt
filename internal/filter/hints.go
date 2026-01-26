package filter

import (
	"fmt"
	"strings"
)

// GetHint returns a single consolidated hint for a parse error.
func GetHint(code ErrorCode, token, query string, position int) string {
	switch code {
	case ErrorCodeUnexpectedToken:
		return getUnexpectedTokenHint(token, query, position)
	case ErrorCodeMissingClosingBracket:
		return getMissingClosingBracketHint(query)
	case ErrorCodeMissingClosingBrace:
		return getMissingClosingBraceHint(query)
	case ErrorCodeMissingGitRef:
		return "Git filters with '...' require a reference on each side. e.g. '[main...HEAD]'"
	case ErrorCodeMissingOperand:
		return ""
	case ErrorCodeUnexpectedEOF:
		return "The expression is incomplete. Make sure all brackets are closed and operators have operands."
	case ErrorCodeIllegalToken:
		return "This character is not recognized. Valid operators: | (union), ! (negation), = (attribute)"

	// These have error messages that are pretty self-explanatory and don't need hints.
	case ErrorCodeEmptyGitFilter, ErrorCodeEmptyExpression:
		return ""

	// These are errors that don't have obvious hints that can be offered.
	case ErrorCodeUnknown:
		return ""
	}

	return ""
}

// GetHints returns context-aware hints for a parse error (legacy, returns slice for compatibility).
func GetHints(code ErrorCode, token, query string, position int) []string {
	hint := GetHint(code, token, query, position)
	if hint == "" {
		return nil
	}

	return []string{hint}
}

// getUnexpectedTokenHint returns a single hint specific to unexpected token errors.
func getUnexpectedTokenHint(token, query string, position int) string {
	switch token {
	case "^":
		return getCaretHint(query, position)
	case "|":
		return ""
	case "=":
		return "The equals sign is used for attribute filters. e.g. 'name=foo'"
	case "]":
		return "Unexpected ']' without matching '['. Git filters use square brackets: '[main...HEAD]'"
	case "}":
		return "Unexpected '}' without matching '{'. Braced paths use braces: '{./my path}'"
	case "...":
		return "The '...' operator must be used in either a graph-based or Git-based expression. e.g. '...foo...' or '[main...HEAD]'"
	}

	// Generic unexpected token hints
	if strings.HasPrefix(token, ".") || strings.HasPrefix(token, "/") {
		return "Path expressions should start with './' for relative or '/' for absolute paths."
	}

	return ""
}

// getCaretHint returns a single hint for caret (^) token errors.
func getCaretHint(query string, position int) string {
	// Check if caret follows text (e.g., "HEAD^")
	if position > 0 {
		beforeCaret := strings.TrimSpace(query[:position])

		// Check if it follows an ellipsis - suggest moving caret to left side
		if targetPart, found := strings.CutSuffix(beforeCaret, "..."); found {
			// Extract the target before the ellipsis for a dynamic suggestion
			return fmt.Sprintf("The '^' operator excludes the target from graph results when used on the left side of the expression. Did you mean '^%s...'?", targetPart)
		}

		// User likely meant Git syntax [HEAD^]
		if len(beforeCaret) > 0 && !strings.ContainsAny(beforeCaret, " \t|!={}[]") {
			return fmt.Sprintf("Git syntax requires '[]'. Did you mean '[%s^]'?", beforeCaret)
		}
	}

	// Caret at start or in unusual position
	return "The '^' operator must be used in either a graph-based or Git-based expression. e.g. '...^foo...' or '[HEAD^]'"
}

// getMissingClosingBracketHint returns a dynamic hint for unclosed Git filter expressions.
func getMissingClosingBracketHint(query string) string {
	// Find the opening bracket and extract content after it
	if _, content, found := strings.Cut(query, "["); found {
		return fmt.Sprintf("Git filter expressions must be enclosed in '[]'. Did you mean '[%s]'?", content)
	}

	return "Git filter expressions must be enclosed in '[]'. e.g. '[main...HEAD]'"
}

// getMissingClosingBraceHint returns a dynamic hint for unclosed braced path expressions.
func getMissingClosingBraceHint(query string) string {
	// Find the opening brace and extract content after it
	if _, content, found := strings.Cut(query, "{"); found {
		return fmt.Sprintf("Braced paths must be enclosed in '{}'. Did you mean '{%s}'?", content)
	}

	return "Braced paths must be enclosed in '{}'. e.g. '{path/with spaces}'"
}
