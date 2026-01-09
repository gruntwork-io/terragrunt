package signal

import (
	"context"
	"errors"
	"os"
)

// ContextCanceledError contains a signal to pass through when the context is cancelled.
type ContextCanceledError struct {
	Signal os.Signal
}

// SignalFromContext extracts the signal that caused the context cancellation, if any.
// Returns nil if the context was not cancelled due to a signal.
func SignalFromContext(ctx context.Context) os.Signal {
	cause := context.Cause(ctx)
	if cause == nil {
		return nil
	}

	var canceledErr *ContextCanceledError
	if errors.As(cause, &canceledErr) && canceledErr.Signal != nil {
		return canceledErr.Signal
	}

	return nil
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
