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
	// Title is a high-level error description (e.g., "Unclosed Git filter expression")
	Title string
	// Message is a detailed explanation shown at the problematic location (e.g., "this Git-based expression is missing a closing ']'")
	Message string
	// Query is the original filter query
	Query string
	// TokenLiteral is the problematic token
	TokenLiteral string
	// TokenLength is the length of the problematic token (used for underline width)
	TokenLength int
	// Position is the position of the problematic token
	Position int
	// ErrorPosition is the position to show the caret (e.g. for unclosed brackets, it points to the opening bracket)
	ErrorPosition int
	// ErrorCode is the error code, used for hint lookup
	ErrorCode ErrorCode
}

// Error returns a string representation of the error.
//
// We suppress the gocritic "hugeParam" warning because this is a very large struct,
// but we need it to implement the error interface, not its pointer.
//
//nolint:gocritic
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
