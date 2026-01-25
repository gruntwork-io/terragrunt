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
		return "Git filter expressions must be enclosed in '[]'. e.g. '[main...HEAD]'"
	case ErrorCodeMissingClosingBrace:
		return "Braced paths must be enclosed in '{}'. e.g. '{path/with spaces}'"
	case ErrorCodeMissingGitRef:
		return "Git filters require at least one reference. e.g. '[main]' or '[main...HEAD]'"
	case ErrorCodeMissingOperand:
		return "Operators require expressions on both sides. e.g. './foo/** | !./foo/bar/**'"
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
		return "The '|' operator requires expressions on both sides. e.g. 'app | !legacy'"
	case "=":
		return "The equals sign is used for attribute filters. e.g. 'name=foo'"
	case "]":
		return "Unexpected ']' without matching '['. Git filters use brackets: '[main...HEAD]'"
	case "}":
		return "Unexpected '}' without matching '{'. Braced paths use braces: '{./my path}'"
	case "...":
		return "Ellipsis is used for graph traversal (e.g. '...foo...') or Git ranges (e.g. '[main...HEAD]')"
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

		// Check if it follows an ellipsis
		if strings.HasSuffix(beforeCaret, "...") {
			return "The caret (^) excludes the target from graph results. e.g. '^foo...' or 'foo...^bar'"
		}

		// User likely meant Git syntax [HEAD^]
		if len(beforeCaret) > 0 && !strings.ContainsAny(beforeCaret, " \t|!={}[]") {
			return fmt.Sprintf("Git syntax requires '[]'. Did you mean '[%s^]'?", beforeCaret)
		}
	}

	// Caret at start or in unusual position
	return "The caret (^) excludes the target from graph results. e.g. '^foo...' or 'foo...^bar'"
}
