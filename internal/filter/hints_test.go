package filter_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHints_GoldenGitSyntaxCaretAfterRef tests the hint when using Git-style caret
// outside of brackets (e.g., HEAD^ instead of [HEAD^]).
func TestHints_GoldenGitSyntaxCaretAfterRef(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("HEAD^")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter 'HEAD^'

     HEAD^
         ^ unexpected '^' after expression

  hint: Git syntax requires '[]'. Did you mean '[HEAD^]'?
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenUnclosedBracket tests the hint for unclosed Git filter brackets.
func TestHints_GoldenUnclosedBracket(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("[main...HEAD")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unclosed Git filter expression
 --> --filter '[main...HEAD'

     [main...HEAD
     ^ This Git-based expression is missing a closing ']'

  hint: Git filter expressions must be enclosed in '[]'. e.g. '[main...HEAD]'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenUnclosedBrace tests the hint for unclosed braced paths.
func TestHints_GoldenUnclosedBrace(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("{my path")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unclosed path expression
 --> --filter '{my path'

     {my path
     ^ This braced path expression is missing a closing '}'

  hint: Braced paths must be enclosed in '{}'. e.g. '{path/with spaces}'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenEmptyGitFilter tests that empty Git filters show no hint.
func TestHints_GoldenEmptyGitFilter(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("[]")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Empty Git filter
 --> --filter '[]'

     []
      ^ Git filter expression cannot be empty

`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenPipeAtStart tests the hint for pipe operator at start of expression.
func TestHints_GoldenPipeAtStart(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("| foo")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter '| foo'

     | foo
     ^ unexpected '|'

  hint: The pipe operator requires expressions on both sides. e.g. 'app | !legacy'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenPipeAtEnd tests the hint for pipe operator at end of expression.
func TestHints_GoldenPipeAtEnd(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("foo |")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected end of input
 --> --filter 'foo |'

     foo |
          ^ expression is incomplete

  hint: The expression is incomplete. Make sure all brackets are closed and operators have operands.
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenBangWithoutOperand tests the hint for negation without operand.
func TestHints_GoldenBangWithoutOperand(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("!")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected end of input
 --> --filter '!'

     !
      ^ expression is incomplete

  hint: The expression is incomplete. Make sure all brackets are closed and operators have operands.
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenUnexpectedClosingBracket tests the hint for unexpected closing bracket.
func TestHints_GoldenUnexpectedClosingBracket(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("]")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter ']'

     ]
     ^ unexpected ']'

  hint: Unexpected ']' without matching '['. Git filters use brackets: '[main...HEAD]'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenUnexpectedClosingBrace tests the hint for unexpected closing brace.
func TestHints_GoldenUnexpectedClosingBrace(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("}")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter '}'

     }
     ^ unexpected '}'

  hint: Unexpected '}' without matching '{'. Braced paths use braces: '{./my path}'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenEqualsWithoutContext tests the hint for equals sign without context.
func TestHints_GoldenEqualsWithoutContext(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("=foo")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter '=foo'

     =foo
     ^ unexpected '='

  hint: The equals sign is used for attribute filters. e.g. 'name=foo'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenCaretAtStart tests the hint for caret at start (graph exclusion).
func TestHints_GoldenCaretAtStart(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("^")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected end of input
 --> --filter '^'

     ^
      ^ expression is incomplete

  hint: The expression is incomplete. Make sure all brackets are closed and operators have operands.
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenEllipsisAtStart tests the hint for ellipsis at start without target.
func TestHints_GoldenEllipsisAtStart(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("...")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected end of input
 --> --filter '...'

     ...
        ^ expression is incomplete

  hint: The expression is incomplete. Make sure all brackets are closed and operators have operands.
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenIllegalCharacter tests the hint for illegal/unrecognized characters.
// A single dot (.) not followed by a path separator or identifier char is illegal.
func TestHints_GoldenIllegalCharacter(t *testing.T) {
	t.Parallel()

	output, err := renderParseError(".")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Illegal token
 --> --filter '.'

     .
     ^ unrecognized character '.'

  hint: This character is not recognized. Valid operators: | (union), ! (negation), = (attribute)
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenMissingGitRefAfterEllipsis tests missing second Git reference.
func TestHints_GoldenMissingGitRefAfterEllipsis(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("[main...]")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Missing Git reference
 --> --filter '[main...]'

     [main...]
             ^ Expected second Git reference after '...'

  hint: Git filters require at least one reference. e.g. '[main]' or '[main...HEAD]'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenComplexExpression tests error in a more complex expression.
func TestHints_GoldenComplexExpression(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("./apps/* | HEAD^")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter './apps/* | HEAD^'

     ./apps/* | HEAD^
                    ^ unexpected '^' after expression

  hint: The caret (^) excludes the target from graph results. e.g. '^foo...' or 'foo...^bar'
`
	assert.Equal(t, expected, output)
}

// TestHints_GoldenCaretAfterEllipsis tests the hint for caret after ellipsis.
func TestHints_GoldenCaretAfterEllipsis(t *testing.T) {
	t.Parallel()

	output, err := renderParseError("./foo...^")
	require.NoError(t, err)

	output = stripTimestampPrefix(output)

	expected := `Filter parsing error: Unexpected token
 --> --filter './foo...^'

     ./foo...^
             ^ unexpected '^' after expression

  hint: The caret (^) excludes the target from graph results. e.g. '^foo...' or 'foo...^bar'
`
	assert.Equal(t, expected, output)
}

// TestHints_ErrorCodeCoverage verifies that all error codes produce appropriate hints.
func TestHints_ErrorCodeCoverage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		token         string
		query         string
		hintSubstring string
		code          filter.ErrorCode
		position      int
		expectHint    bool
	}{
		{
			name:          "UnexpectedToken with pipe",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "|",
			query:         "| foo",
			position:      0,
			expectHint:    true,
			hintSubstring: "both sides",
		},
		{
			name:          "UnexpectedToken with caret",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "^",
			query:         "HEAD^",
			position:      4,
			expectHint:    true,
			hintSubstring: "Git",
		},
		{
			name:          "UnexpectedToken with equals",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "=",
			query:         "=foo",
			position:      0,
			expectHint:    true,
			hintSubstring: "attribute",
		},
		{
			name:          "UnexpectedToken with closing bracket",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "]",
			query:         "]",
			position:      0,
			expectHint:    true,
			hintSubstring: "without matching '['",
		},
		{
			name:          "UnexpectedToken with closing brace",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "}",
			query:         "}",
			position:      0,
			expectHint:    true,
			hintSubstring: "without matching '{'",
		},
		{
			name:          "UnexpectedToken with ellipsis",
			code:          filter.ErrorCodeUnexpectedToken,
			token:         "...",
			query:         "...",
			position:      0,
			expectHint:    true,
			hintSubstring: "graph traversal",
		},
		{
			name:          "MissingClosingBracket",
			code:          filter.ErrorCodeMissingClosingBracket,
			token:         "",
			query:         "[main",
			position:      5,
			expectHint:    true,
			hintSubstring: "enclosed in '[]'",
		},
		{
			name:          "MissingClosingBrace",
			code:          filter.ErrorCodeMissingClosingBrace,
			token:         "",
			query:         "{path",
			position:      5,
			expectHint:    true,
			hintSubstring: "enclosed in '{}'",
		},
		{
			name:          "MissingGitRef",
			code:          filter.ErrorCodeMissingGitRef,
			token:         "",
			query:         "[main...]",
			position:      8,
			expectHint:    true,
			hintSubstring: "require at least one reference",
		},
		{
			name:          "MissingOperand",
			code:          filter.ErrorCodeMissingOperand,
			token:         "",
			query:         "foo |",
			position:      5,
			expectHint:    true,
			hintSubstring: "both sides",
		},
		{
			name:          "UnexpectedEOF",
			code:          filter.ErrorCodeUnexpectedEOF,
			token:         "",
			query:         "...",
			position:      3,
			expectHint:    true,
			hintSubstring: "incomplete",
		},
		{
			name:          "IllegalToken",
			code:          filter.ErrorCodeIllegalToken,
			token:         "@",
			query:         "@",
			position:      0,
			expectHint:    true,
			hintSubstring: "not recognized",
		},
		{
			name:       "EmptyGitFilter - no hint",
			code:       filter.ErrorCodeEmptyGitFilter,
			token:      "]",
			query:      "[]",
			position:   1,
			expectHint: false,
		},
		{
			name:       "EmptyExpression - no hint",
			code:       filter.ErrorCodeEmptyExpression,
			token:      "}",
			query:      "{}",
			position:   1,
			expectHint: false,
		},
		{
			name:       "Unknown - no hint",
			code:       filter.ErrorCodeUnknown,
			token:      "",
			query:      "",
			position:   0,
			expectHint: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hint := filter.GetHint(tc.code, tc.token, tc.query, tc.position)

			if tc.expectHint {
				require.NotEmpty(t, hint, "expected hint for error code %v", tc.code)
				assert.Contains(t, hint, tc.hintSubstring,
					"hint should contain '%s', got: %s", tc.hintSubstring, hint)
			} else {
				assert.Empty(t, hint, "expected no hint for error code %v", tc.code)
			}
		})
	}
}

// TestHints_CaretContextualHints tests that caret hints vary based on context.
func TestHints_CaretContextualHints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		query         string
		hintSubstring string
		position      int
	}{
		{
			name:          "caret after identifier suggests Git syntax",
			query:         "HEAD^",
			position:      4,
			hintSubstring: "[HEAD^]",
		},
		{
			name:          "caret after ellipsis suggests graph exclusion",
			query:         "foo...^bar",
			position:      6,
			hintSubstring: "excludes the target",
		},
		{
			name:          "caret at start suggests graph exclusion",
			query:         "^foo",
			position:      0,
			hintSubstring: "excludes the target",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hint := filter.GetHint(filter.ErrorCodeUnexpectedToken, "^", tc.query, tc.position)

			require.NotEmpty(t, hint)
			assert.Contains(t, hint, tc.hintSubstring)
		})
	}
}

// TestHints_FormatDiagnosticStructure verifies the overall structure of diagnostic output.
func TestHints_FormatDiagnosticStructure(t *testing.T) {
	t.Parallel()

	parseErr := &filter.ParseError{
		Title:         "Test Error",
		Message:       "test message",
		Position:      5,
		ErrorPosition: 5,
		Query:         "test query",
		TokenLiteral:  "q",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeMissingClosingBracket,
	}

	output := filter.FormatDiagnostic(parseErr, 0, false)

	// Verify structural elements are present and in order
	lines := strings.Split(output, "\n")

	require.GreaterOrEqual(t, len(lines), 6, "diagnostic should have at least 6 lines")

	// Line 1: Error header
	assert.Contains(t, lines[0], "Filter parsing error:")
	assert.Contains(t, lines[0], "Test Error")

	// Line 2: Location arrow
	assert.Contains(t, lines[1], " --> ")
	assert.Contains(t, lines[1], "--filter")

	// Line 3: Blank line
	assert.Empty(t, lines[2])

	// Line 4: Query
	assert.Contains(t, lines[3], "test query")

	// Line 5: Caret and message
	assert.Contains(t, lines[4], "^")
	assert.Contains(t, lines[4], "test message")

	// Line 6: Blank line
	assert.Empty(t, lines[5])

	// Line 7: Hint (when present)
	assert.Contains(t, lines[6], "hint:")
}

// TestHints_FilterIndexInDiagnostic verifies filter index appears in multi-filter scenarios.
func TestHints_FilterIndexInDiagnostic(t *testing.T) {
	t.Parallel()

	parseErr := &filter.ParseError{
		Title:         "Test Error",
		Message:       "test message",
		Position:      0,
		ErrorPosition: 0,
		Query:         "bad",
		TokenLiteral:  "b",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeUnexpectedToken,
	}

	// Filter index 0 should not show index
	output0 := filter.FormatDiagnostic(parseErr, 0, false)
	assert.Contains(t, output0, "--filter 'bad'")
	assert.NotContains(t, output0, "--filter[")

	// Filter index > 0 should show index
	output2 := filter.FormatDiagnostic(parseErr, 2, false)
	assert.Contains(t, output2, "--filter[2]")
}

// stripTimestampPrefix removes any timestamp prefix from log output.
// Timestamps typically appear at the start of lines in formats like:
// "2024-01-15T10:30:00Z" or "2024/01/15 10:30:00"
func stripTimestampPrefix(s string) string {
	// Match common timestamp patterns at the start of lines
	timestampPattern := regexp.MustCompile(`(?m)^(\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}[^\s]*\s+)`)
	return timestampPattern.ReplaceAllString(s, "")
}

// renderParseError parses a filter query and returns the formatted diagnostic.
// Returns an error if parsing succeeds (no error to render).
func renderParseError(query string) (string, error) {
	_, err := filter.Parse(query)
	if err == nil {
		return "", errors.New("expected parse error but got none")
	}

	var parseErr filter.ParseError
	if !errors.As(err, &parseErr) {
		return "", errors.Errorf("expected ParseError but got: %v", err)
	}

	// Render without colors for consistent golden testing
	return filter.FormatDiagnostic(&parseErr, 0, false), nil
}
