package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLoggerForDiagnostics creates a logger for tests with colors disabled.
func testLoggerForDiagnostics() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestFormatDiagnostic_UnexpectedToken(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Unexpected token",
		Message:       "unexpected '^' after expression",
		Position:      4,
		ErrorPosition: 4,
		Query:         "HEAD^",
		TokenLiteral:  "^",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check error header with title
	assert.Contains(t, result, "Filter parsing error: Unexpected token")

	// Check location arrow
	assert.Contains(t, result, " --> --filter 'HEAD^'")

	// Check query is displayed
	assert.Contains(t, result, "     HEAD^")

	// Check caret at ErrorPosition with message
	assert.Contains(t, result, "^ unexpected '^' after expression")

	// Check hint is present
	assert.Contains(t, result, "hint:")
	assert.Contains(t, result, "Git")
}

func TestFormatDiagnostic_WithFilterIndex(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Unexpected token",
		Message:       "unexpected '|'",
		Position:      0,
		ErrorPosition: 0,
		Query:         "| foo",
		TokenLiteral:  "|",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 2, false)

	// Check filter index is included
	assert.Contains(t, result, " --> --filter[2] '| foo'")
}

func TestFormatDiagnostic_MissingClosingBracket(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Unclosed Git filter expression",
		Message:       "this Git-based expression is missing a closing ']'",
		Position:      12,
		ErrorPosition: 0, // Points to opening bracket
		Query:         "[main...HEAD",
		TokenLiteral:  "",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeMissingClosingBracket,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check error header with title
	assert.Contains(t, result, "Filter parsing error: Unclosed Git filter expression")

	// Check caret points to opening bracket (position 0)
	assert.Contains(t, result, "     ^ this Git-based expression is missing a closing ']'")

	// Check consolidated hint
	assert.Contains(t, result, "hint: Git-based expressions require surrounding references with '[]'")
}

func TestFormatDiagnostic_EmptyGitFilter(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Empty Git filter",
		Message:       "Git filter expression cannot be empty",
		Position:      1,
		ErrorPosition: 1,
		Query:         "[]",
		TokenLiteral:  "]",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeEmptyGitFilter,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	assert.Contains(t, result, "Filter parsing error: Empty Git filter")

	assert.NotContains(t, result, "hint:")
}

func TestFormatDiagnostic_WithColor(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Unexpected token",
		Message:       "unexpected '^' after expression",
		Position:      4,
		ErrorPosition: 4,
		Query:         "HEAD^",
		TokenLiteral:  "^",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, true)

	// Check ANSI codes are present
	assert.Contains(t, result, "\033[") // ANSI escape sequence
}

func TestFormatDiagnostic_NoColor(t *testing.T) {
	t.Parallel()

	err := &filter.ParseError{
		Title:         "Unexpected token",
		Message:       "unexpected '^' after expression",
		Position:      4,
		ErrorPosition: 4,
		Query:         "HEAD^",
		TokenLiteral:  "^",
		TokenLength:   1,
		ErrorCode:     filter.ErrorCodeUnexpectedToken,
	}

	result := filter.FormatDiagnostic(err, 0, false)

	// Check no ANSI codes
	assert.NotContains(t, result, "\033[")
}

func TestGetHint_CaretAfterIdentifier(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeUnexpectedToken, "^", "HEAD^", 4)

	require.NotEmpty(t, hint)
	assert.Contains(t, hint, "Git")
	assert.Contains(t, hint, "[HEAD^]")
}

func TestGetHint_CaretAtStart(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeUnexpectedToken, "^", "^foo", 0)

	require.NotEmpty(t, hint)
	assert.Contains(t, hint, "excludes the target")
}

func TestGetHint_MissingClosingBracket(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeMissingClosingBracket, "", "[main...HEAD", 12)

	require.NotEmpty(t, hint)
	assert.Contains(t, hint, "[]")
}

func TestGetHint_MissingClosingBrace(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeMissingClosingBrace, "", "{my path", 8)

	require.NotEmpty(t, hint)
	assert.Contains(t, hint, "{}")
}

func TestGetHint_EmptyGitFilter(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeEmptyGitFilter, "]", "[]", 1)

	assert.Empty(t, hint)
}

func TestGetHint_PipeOperator(t *testing.T) {
	t.Parallel()

	hint := filter.GetHint(filter.ErrorCodeUnexpectedToken, "|", "| foo", 0)

	// Pipe errors have specific messages that are self-explanatory, no hint needed
	assert.Empty(t, hint)
}

func TestParseFilterQueries_RichDiagnostics(t *testing.T) {
	t.Parallel()

	_, err := filter.ParseFilterQueries(testLoggerForDiagnostics(), []string{"HEAD^"})

	require.Error(t, err)

	errMsg := err.Error()

	// Check error structure
	assert.Contains(t, errMsg, "error:")
	assert.Contains(t, errMsg, " --> ")
	assert.Contains(t, errMsg, "HEAD^")
	assert.Contains(t, errMsg, "^")
	assert.Contains(t, errMsg, "hint:")
}

func TestParseFilterQueries_MultipleErrors(t *testing.T) {
	t.Parallel()

	_, err := filter.ParseFilterQueries(testLoggerForDiagnostics(), []string{"HEAD^", "[unclosed"})

	require.Error(t, err)

	errMsg := err.Error()

	// Check both errors are present
	// First filter (index 0) shows as "--filter 'HEAD^'" without index
	assert.Contains(t, errMsg, "--filter 'HEAD^'")
	// Second filter (index 1) shows as "--filter[1]"
	assert.Contains(t, errMsg, "--filter[1]")
	assert.Contains(t, errMsg, "unclosed")
}

func TestParseFilterQueries_ValidFilters(t *testing.T) {
	t.Parallel()

	filters, err := filter.ParseFilterQueries(testLoggerForDiagnostics(), []string{"name=foo", "./apps/*"})

	require.NoError(t, err)
	assert.Len(t, filters, 2)
}

func TestParseFilterQueries_EmptyInput(t *testing.T) {
	t.Parallel()

	filters, err := filter.ParseFilterQueries(testLoggerForDiagnostics(), []string{})

	require.NoError(t, err)
	assert.Empty(t, filters)
}
