package signal

import (
	"context"
	"os"
)

// ContextCanceledError contains a signal to pass through when the context is cancelled.
type ContextCanceledError struct {
	Signal os.Signal
}

// NewContextCanceledError returns a new `ContextCanceledError` instance.
func NewContextCanceledError(sig os.Signal) *ContextCanceledError {
	return &ContextCanceledError{Signal: sig}
}

// Error implements the `Error` method.
func (ContextCanceledError) Error() string {
	return context.Canceled.Error()
}

// Unwrap implements the `Unwrap` method.
func (ContextCanceledError) Unwrap() error {
	return context.Canceled
}
