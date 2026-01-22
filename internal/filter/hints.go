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
			"e.g. '[main...HEAD]'",
		}
	case ErrorCodeMissingClosingBrace:
		return []string{
			"Braced paths must be closed with '}'.",
			"e.g. '{my path with spaces}'",
		}
	case ErrorCodeEmptyGitFilter:
		return []string{
			"Git filter cannot be empty.",
			"e.g. '[main...HEAD]' or '[main]'",
		}
	case ErrorCodeEmptyExpression:
		return []string{
			"Braced path expression cannot be empty.",
			"e.g. '{./my path}'",
		}
	case ErrorCodeMissingGitRef:
		return []string{
			"Git filters require at least one reference.",
			"e.g. '[main]' or '[main...HEAD]'",
		}
	case ErrorCodeMissingOperand:
		return []string{
			"Operators require expressions on both sides.",
			"e.g. './foo/** | ./foo/bar/**' filters for all units in ./foo except ./foo/bar.",
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
			"e.g. './foo/** | !./foo/bar/**' filters for all units in ./foo except ./foo/bar.",
		}
	case "=":
		return []string{
			"The equals sign is used for attribute filters.",
			"e.g. 'name=foo'  filters for all units with the 'foo' name.",
		}
	case "]":
		return []string{
			"Unexpected closing bracket for a Git expression without matching '['.",
			"e.g. '[main...HEAD]' filters for all units that changed between main and HEAD.",
		}
	case "}":
		return []string{
			"Unexpected closing brace for a braced path without matching '{'.",
			"e.g. '{./my path}' filters for a unit named 'my path' in the current directory.",
		}
	case "...":
		return []string{
			"Ellipsis '...' is used for graph traversal and graph expressions.",
			"e.g. '...foo...' filters for all dependencies and dependents of foo.",
			"e.g. '[main...HEAD]' filters for all units that changed between main and HEAD.",
		}
	}

	// Generic unexpected token hints
	if strings.HasPrefix(token, ".") || strings.HasPrefix(token, "/") {
		return []string{
			"Path expressions should start with './' for relative or '/' for absolute paths.",
			"e.g. './relative/path' or '/absolute/path'",
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
				"e.g. '^...foo' selects dependents of foo, excluding foo itself.",
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
		"e.g. '^...foo' selects dependents of foo, excluding foo itself.",
	}
}
