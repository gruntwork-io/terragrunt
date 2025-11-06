package filter

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ParseError represents an error that occurred during parsing.
type ParseError struct {
	Message  string
	Position int
}

func (e ParseError) Error() string {
	return fmt.Sprintf("parse error at position %d: %s", e.Position, e.Message)
}

// NewParseError creates a new ParseError with the given message and position.
func NewParseError(message string, position int) error {
	return errors.New(ParseError{Message: message, Position: position})
}

// EvaluationError represents an error that occurred during evaluation.
type EvaluationError struct {
	Cause   error
	Message string
}

func (e EvaluationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("evaluation error: %s: %v", e.Message, e.Cause)
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
