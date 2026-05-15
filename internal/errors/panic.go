package errors

import (
	"fmt"
	"runtime/debug"
)

// FunctionPanicError wraps a value recovered from a panic, together with the
// stack trace captured at recovery time. It is the canonical type emitted by
// Recover and by helpers in this package, so downstream code can detect
// recovered panics with a typed assertion or via IsFunctionPanic.
type FunctionPanicError struct {
	Recovered any
	Stack     string
}

// NewFunctionPanicError wraps a recovered value and captures the current
// goroutine's stack via runtime/debug.Stack. Always call this from inside a
// deferred recover so the captured frames point at the panic origin.
func NewFunctionPanicError(recovered any) FunctionPanicError {
	return FunctionPanicError{
		Recovered: recovered,
		Stack:     string(debug.Stack()),
	}
}

// Error returns the error message for the panic.
func (err FunctionPanicError) Error() string {
	return fmt.Sprintf("panic in function implementation: %v", err.Recovered)
}

// ErrorStack returns the stack trace associated with the panic.
func (err FunctionPanicError) ErrorStack() string {
	return err.Stack
}

// IsFunctionPanic reports whether this error represents a function panic.
func (FunctionPanicError) IsFunctionPanic() bool {
	return true
}
