package filter_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDiagnostic_UnexpectedToken(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "unexpected token after expression: ^",
		Position:     4,
		Query:        "HEAD^",
		TokenLiteral: "^",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check error header
	assert.Contains(t, result, "error: unexpected token after expression: ^")

	// Check location arrow
	assert.Contains(t, result, " --> --filter 'HEAD^'")

	// Check query is displayed
	assert.Contains(t, result, "     HEAD^")

	// Check underline position (4 spaces + 5 indent = 9 chars before ^)
	assert.Contains(t, result, "         ^ unexpected token")

	// Check hints are present
	assert.Contains(t, result, "hint:")
	assert.Contains(t, result, "Git")
}

func TestFormatDiagnostic_WithFilterIndex(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "Unexpected token: |",
		Position:     0,
		Query:        "| foo",
		TokenLiteral: "|",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 2, false)

	// Check filter index is included
	assert.Contains(t, result, " --> --filter[2] '| foo'")
}

func TestFormatDiagnostic_MissingClosingBracket(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "expected ']' to close Git filter",
		Position:     12,
		Query:        "[main...HEAD",
		TokenLiteral: "",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeMissingClosingBracket,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check error message
	assert.Contains(t, result, "Filter parsing error: expected ']' to close Git filter")

	// Check hints
	assert.Contains(t, result, "hint: Git filter expressions must be closed with ']'")
}

func TestFormatDiagnostic_EmptyGitFilter(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "empty Git filter expression",
		Position:     1,
		Query:        "[]",
		TokenLiteral: "]",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeEmptyGitFilter,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check error message
	assert.Contains(t, result, "Filter parsing error: empty Git filter expression")

	// Check hints
	assert.Contains(t, result, "hint: Git filter cannot be empty")
}

func TestFormatDiagnostic_WithColor(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "Unexpected token after expression: ^",
		Position:     4,
		Query:        "HEAD^",
		TokenLiteral: "^",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, true)

	// Check ANSI codes are present
	assert.Contains(t, result, "\033[") // ANSI escape sequence
}

func TestFormatDiagnostic_NoColor(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Message:      "Unexpected token after expression: ^",
		Position:     4,
		Query:        "HEAD^",
		TokenLiteral: "^",
		TokenLength:  1,
		ErrorCode:    filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check no ANSI codes
	assert.NotContains(t, result, "\033[")
}

func TestGetHints_CaretAfterIdentifier(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeUnexpectedToken, "^", "HEAD^", 4)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "Git")
	assert.Contains(t, strings.Join(hints, " "), "[HEAD^]")
}

func TestGetHints_CaretAtStart(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeUnexpectedToken, "^", "^foo", 0)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "excludes the target")
}

func TestGetHints_MissingClosingBracket(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeMissingClosingBracket, "", "[main...HEAD", 12)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "]")
}

func TestGetHints_MissingClosingBrace(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeMissingClosingBrace, "", "{my path", 8)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "}")
}

func TestGetHints_EmptyGitFilter(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeEmptyGitFilter, "]", "[]", 1)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "empty")
}

func TestGetHints_PipeOperator(t *testing.T) {
	t.Parallel()

	hints := filter.GetHints(filter.ErrorCodeUnexpectedToken, "|", "| foo", 0)

	require.NotEmpty(t, hints)
	assert.Contains(t, strings.Join(hints, " "), "both sides")
}

func TestParseFilterQueriesWithColor_RichDiagnostics(t *testing.T) {
	t.Parallel()

	_, err := filter.ParseFilterQueriesWithColor([]string{"HEAD^"}, false)

	require.Error(t, err)

	errMsg := err.Error()

	// Check error structure
	assert.Contains(t, errMsg, "error:")
	assert.Contains(t, errMsg, " --> ")
	assert.Contains(t, errMsg, "HEAD^")
	assert.Contains(t, errMsg, "^")
	assert.Contains(t, errMsg, "hint:")
}

func TestParseFilterQueriesWithColor_MultipleErrors(t *testing.T) {
	t.Parallel()

	_, err := filter.ParseFilterQueriesWithColor([]string{"HEAD^", "[unclosed"}, false)

	require.Error(t, err)

	errMsg := err.Error()

	// Check both errors are present
	// First filter (index 0) shows as "--filter 'HEAD^'" without index
	assert.Contains(t, errMsg, "--filter 'HEAD^'")
	// Second filter (index 1) shows as "--filter[1]"
	assert.Contains(t, errMsg, "--filter[1]")
	assert.Contains(t, errMsg, "unclosed")
}

func TestParseFilterQueriesWithColor_ValidFilters(t *testing.T) {
	t.Parallel()

	filters, err := filter.ParseFilterQueriesWithColor([]string{"name=foo", "./apps/*"}, false)

	require.NoError(t, err)
	assert.Len(t, filters, 2)
}

func TestParseFilterQueriesWithColor_EmptyInput(t *testing.T) {
	t.Parallel()

	filters, err := filter.ParseFilterQueriesWithColor([]string{}, false)

	require.NoError(t, err)
	assert.Empty(t, filters)
}

func TestParseFilterQueriesWithColor_WithColor(t *testing.T) {
	t.Parallel()

	_, err := filter.ParseFilterQueriesWithColor([]string{"HEAD^"}, true)

	require.Error(t, err)

	errMsg := err.Error()

	// Check ANSI codes are present
	assert.Contains(t, errMsg, "\033[")
}
