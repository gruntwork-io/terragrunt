package filter

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ErrorCode categorizes parse errors for hint lookup.
type ErrorCode int

const (
	ErrorCodeUnknown ErrorCode = iota
	ErrorCodeUnexpectedToken
	ErrorCodeUnexpectedEOF
	ErrorCodeEmptyExpression
	ErrorCodeMissingClosingBracket
	ErrorCodeMissingClosingBrace
	ErrorCodeIllegalToken
	ErrorCodeMissingOperand
	ErrorCodeEmptyGitFilter
	ErrorCodeMissingGitRef
)

// ParseError represents an error that occurred during parsing.
type ParseError struct {
	Title         string    // High-level error description (e.g., "Unclosed Git filter expression")
	Message       string    // Detailed explanation (shown at caret position)
	Position      int       // Position of the problematic token (current behavior)
	ErrorPosition int       // Position to show the caret (may differ from Position for unclosed brackets)
	Query         string    // Original filter query
	TokenLiteral  string    // The problematic token
	TokenLength   int       // For underline width
	ErrorCode     ErrorCode // For hint lookup
}

func (e ParseError) Error() string {
	return fmt.Sprintf("Parse error at position %d: %s", e.Position, e.Message)
}

// NewParseError creates a new ParseError with the given message and position.
func NewParseError(message string, position int) error {
	return errors.New(ParseError{Message: message, Position: position})
}

// NewParseErrorWithContext creates a new ParseError with full context for rich diagnostics.
func NewParseErrorWithContext(title, message string, position, errorPosition int, query, tokenLiteral string, tokenLength int, code ErrorCode) error {
	return errors.New(ParseError{
		Title:         title,
		Message:       message,
		Position:      position,
		ErrorPosition: errorPosition,
		Query:         query,
		TokenLiteral:  tokenLiteral,
		TokenLength:   tokenLength,
		ErrorCode:     code,
	})
}

// EvaluationError represents an error that occurred during evaluation.
type EvaluationError struct {
	Cause   error
	Message string
}

func (e EvaluationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("Evaluation error: %s: %v", e.Message, e.Cause)
	}

	return "evaluation error: " + e.Message
}

// NewEvaluationError creates a new EvaluationError with the given message.
func NewEvaluationError(message string) error {
	return errors.New(EvaluationError{Message: message})
}

// NewEvaluationErrorWithCause creates a new EvaluationError with the given message and cause.
func NewEvaluationErrorWithCause(message string, cause error) error {
	return errors.New(EvaluationError{Message: message, Cause: cause})
}

// FilterQueryRequiresDiscoveryError is an error that is returned when a filter query requires discovery of Terragrunt configurations.
type FilterQueryRequiresDiscoveryError struct {
	Query string
}

func (e FilterQueryRequiresDiscoveryError) Error() string {
	return fmt.Sprintf(
		"Filter query '%s' requires discovery of Terragrunt configurations, which is not supported when evaluating filters on generic files",
		e.Query,
	)
}
