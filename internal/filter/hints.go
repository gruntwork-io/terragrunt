package filter

import (
	"fmt"
	"strings"
)

// GetHints returns context-aware hints for a parse error.
func GetHints(code ErrorCode, token, query string, position int) []string {
	switch code {
	case ErrorCodeUnexpectedToken:
		return getUnexpectedTokenHints(token, query, position)
	case ErrorCodeMissingClosingBracket:
		return []string{
			"Git filter expressions must be closed with ']'.",
			"Example: '[main...HEAD]'",
		}
	case ErrorCodeMissingClosingBrace:
		return []string{
			"Braced paths must be closed with '}'.",
			"Example: '{my path with spaces}'",
		}
	case ErrorCodeEmptyGitFilter:
		return []string{
			"Git filter cannot be empty.",
			"Example: '[main...HEAD]' or '[main]'",
		}
	case ErrorCodeEmptyExpression:
		return []string{
			"Braced path expression cannot be empty.",
			"Example: '{./my path}'",
		}
	case ErrorCodeMissingGitRef:
		return []string{
			"Git filters require at least one reference.",
			"Example: '[main]' or '[main...HEAD]'",
		}
	case ErrorCodeMissingOperand:
		return []string{
			"Operators require expressions on both sides.",
			"Example: 'foo | bar' or '!name=excluded'",
		}
	case ErrorCodeUnexpectedEOF:
		return []string{
			"The expression is incomplete.",
			"Make sure all brackets are closed and operators have operands.",
		}
	case ErrorCodeIllegalToken:
		return []string{
			"This character is not recognized in filter expressions.",
			"Valid operators: | (union), ! (negation), = (attribute)",
		}
	}

	return nil
}

// getUnexpectedTokenHints returns hints specific to unexpected token errors.
func getUnexpectedTokenHints(token, query string, position int) []string {
	switch token {
	case "^":
		return getCaretHints(query, position)
	case "|":
		return []string{
			"The pipe operator requires expressions on both sides.",
			"Example: 'a | b' selects both 'a' and 'b'.",
		}
	case "=":
		return []string{
			"The equals sign is used for attribute filters.",
			"Example: 'name=foo' or 'tag=production'",
		}
	case "]":
		return []string{
			"Unexpected closing bracket without matching '['.",
			"Git filters use brackets: '[main...HEAD]'",
		}
	case "}":
		return []string{
			"Unexpected closing brace without matching '{'.",
			"Braced paths use braces: '{my path}'",
		}
	case "...":
		return []string{
			"Ellipsis (...) is used for graph traversal.",
			"Prefix: '...foo' selects dependents of foo.",
			"Suffix: 'foo...' selects dependencies of foo.",
		}
	}

	// Generic unexpected token hints
	if strings.HasPrefix(token, ".") || strings.HasPrefix(token, "/") {
		return []string{
			"Paths should start with './' for relative or '/' for absolute.",
			"Example: './apps/*' or '/absolute/path'",
		}
	}

	return nil
}

// getCaretHints returns hints for caret (^) token errors.
func getCaretHints(query string, position int) []string {
	// Check if caret follows text (e.g., "HEAD^")
	if position > 0 {
		beforeCaret := strings.TrimSpace(query[:position])

		// Check if it follows an ellipsis
		if strings.HasSuffix(beforeCaret, "...") {
			return []string{
				"The caret (^) excludes the target from graph results.",
				"It must follow the ellipsis, not precede text.",
				"Example: '^...foo' selects dependents of foo, excluding foo itself.",
			}
		}

		// User likely meant Git syntax [HEAD^]
		if len(beforeCaret) > 0 && !strings.ContainsAny(beforeCaret, " \t|!={}[]") {
			return []string{
				"It looks like you're trying to use Git revision syntax.",
				fmt.Sprintf("Did you mean '[%s^]' (Git parent reference)?", beforeCaret),
				"Git filters must be enclosed in brackets: [ref] or [from...to]",
			}
		}
	}

	// Caret at start or in unusual position
	return []string{
		"The caret (^) excludes the target from graph results.",
		"It must be combined with ellipsis (...) for graph traversal.",
		"Example: '^...foo' selects dependents of foo, excluding foo itself.",
	}
}
