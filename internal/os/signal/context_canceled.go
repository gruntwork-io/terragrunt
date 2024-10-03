package signal

import (
	"context"
	"os"
)

// ContextCanceledCause contains a signal to pass through when the context is cancelled.
type ContextCanceledCause struct {
	Signal os.Signal
}

// NewContextCanceledCause returns a new `ContextCanceledCause` instance.
func NewContextCanceledCause(sig os.Signal) *ContextCanceledCause {
	return &ContextCanceledCause{Signal: sig}
}

// Error implements the `Error` method.
func (ContextCanceledCause) Error() string {
	return context.Canceled.Error()
}

// Unwrap implements the `Unwrap` method.
func (ContextCanceledCause) Unwrap() error {
	return context.Canceled
}
