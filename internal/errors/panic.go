package errors

import "fmt"

// FunctionPanicError wraps a panic recovered from a function wrapper with a stack trace.
type FunctionPanicError struct {
	Recovered any
	Stack     string
}

func (err FunctionPanicError) Error() string {
	return fmt.Sprintf("panic in function implementation: %v", err.Recovered)
}

func (err FunctionPanicError) ErrorStack() string {
	return err.Stack
}

func (err FunctionPanicError) IsFunctionPanic() bool {
	return true
}
