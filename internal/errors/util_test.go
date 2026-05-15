package errors_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

func TestIsFunctionPanic(t *testing.T) {
	t.Parallel()

	t.Run("custom function panic marker", func(t *testing.T) {
		t.Parallel()

		err := functionPanicLikeError{
			Recovered: "slice bounds out of range",
			Stack:     "stack one",
		}

		assert.True(t, errors.IsFunctionPanic(err))
	})

	t.Run("cty function panic", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{
			Value: "panic message",
			Stack: []byte("cty stack"),
		}

		assert.True(t, errors.IsFunctionPanic(err))
		assert.Equal(t, "cty stack", errors.ErrorStack(err))
	})

	t.Run("FunctionPanicError from this package", func(t *testing.T) {
		t.Parallel()

		err := errors.FunctionPanicError{
			Recovered: "runtime nil deref",
			Stack:     "panic-stack",
		}

		assert.True(t, errors.IsFunctionPanic(err))
		assert.Equal(t, "panic-stack", errors.ErrorStack(err))
	})

	t.Run("cty function panic wrapped error", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{
			Value: "panic message",
			Stack: []byte("cty stack"),
		}
		wrapped := fmt.Errorf("wrapped: %w", err)

		assert.True(t, errors.IsFunctionPanic(wrapped))
	})

	t.Run("hcl diagnostic function call panic", func(t *testing.T) {
		t.Parallel()

		diag := &hcl.Diagnostic{
			Extra: fakeFunctionCallDiagExtra{
				functionErr: function.PanicError{
					Value: "panic in function implementation",
					Stack: []byte("diag cty stack"),
				},
			},
		}
		err := hcl.Diagnostics{diag}

		assert.True(t, errors.IsFunctionPanic(err))
		assert.True(t, errors.IsFunctionPanic(errors.New(diag)))
		assert.True(t, errors.IsFunctionPanic(fmt.Errorf("wrapped: %w", err)))
	})

	// Regression test: previously isFunctionPanic returned true for any
	// errors.New(...)-wrapped error because go-errors auto-attaches a stack
	// and the heuristic treated any present stack as evidence of a panic.
	// Plain wrapped errors must NOT be classified as panics or every error
	// will route through the crash-report UX.
	t.Run("plain wrapped error is not a function panic", func(t *testing.T) {
		t.Parallel()

		assert.False(t, errors.IsFunctionPanic(errors.New("a regular failure")))
		assert.False(t, errors.IsFunctionPanic(errors.Errorf("formatted: %d", 42)))
		assert.False(t, errors.IsFunctionPanic(errors.New("panic: this string mentions panic but is not one")))
	})

	t.Run("non panic error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, errors.IsFunctionPanic(assert.AnError))
	})
}

func TestErrorStackForFunctionPanic(t *testing.T) {
	t.Parallel()

	t.Run("includes function panic stack", func(t *testing.T) {
		t.Parallel()

		err := functionPanicLikeError{
			Recovered: "panic",
			Stack:     "custom stack",
		}

		assert.Equal(t, "custom stack", errors.ErrorStack(err))
	})

	t.Run("extracts cty function stack", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{
			Value: "panic message",
			Stack: []byte("cty debug stack"),
		}

		assert.Equal(t, "cty debug stack", errors.ErrorStack(err))
	})

	t.Run("extracts cty function stack from wrapped error", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{
			Value: "panic message",
			Stack: []byte("cty debug stack"),
		}
		wrapped := fmt.Errorf("wrapped: %w", err)

		assert.Contains(t, errors.ErrorStack(wrapped), "cty debug stack")
	})

	t.Run("extracts cty function stack from hcl function call diagnostic", func(t *testing.T) {
		t.Parallel()

		panicStack := "diag cty stack"
		err := errors.New(&hcl.Diagnostic{
			Extra: fakeFunctionCallDiagExtra{
				functionErr: function.PanicError{
					Value: "runtime panic",
					Stack: []byte(panicStack),
				},
			},
		})

		crashOutput := errors.ErrorStack(err)
		assert.Contains(t, crashOutput, panicStack)
	})

	// Regression test: ErrorStack used to invoke both the marker-interface
	// branch and a reflection-based field walker, appending the same stack
	// twice for any go-errors wrapped error.
	t.Run("does not duplicate stacks for wrapped errors", func(t *testing.T) {
		t.Parallel()

		err := errors.New("wrapped err")
		stack := errors.ErrorStack(err)

		require.NotEmpty(t, stack)
		// "main.main" or "testing.tRunner" appears in any goroutine stack;
		// a duplicated stack contains exactly one repetition of any frame.
		count := 0
		needle := "errors_test.TestErrorStackForFunctionPanic"

		for i := 0; i+len(needle) <= len(stack); i++ {
			if stack[i:i+len(needle)] == needle {
				count++
			}
		}
		// Duplicated stack would push the count past 1.
		assert.LessOrEqual(t, count, 1, "ErrorStack should not duplicate frames; got %d copies", count)
	})
}

func TestRecoverWrapsPanic(t *testing.T) {
	t.Parallel()

	t.Run("recovers non-error panic as function panic", func(t *testing.T) {
		t.Parallel()

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic("panic-value")
		}()

		require.Error(t, recovered)
		assert.True(t, errors.IsFunctionPanic(recovered))
		assert.Contains(t, recovered.Error(), "panic")
		assert.NotEmpty(t, errors.ErrorStack(recovered))
	})

	t.Run("preserves function panic error from recover", func(t *testing.T) {
		t.Parallel()

		panicErr := errors.FunctionPanicError{
			Recovered: "runtime error",
			Stack:     "existing function panic stack",
		}

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic(panicErr)
		}()

		assert.True(t, errors.IsFunctionPanic(recovered))
		assert.Equal(t, "existing function panic stack", errors.ErrorStack(recovered))
	})

	t.Run("wraps error-typed panic value as function panic", func(t *testing.T) {
		t.Parallel()

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic(fmt.Errorf("boom"))
		}()

		require.Error(t, recovered)
		assert.True(t, errors.IsFunctionPanic(recovered))
	})

	// Regression test: invoking errors.Recover indirectly through another
	// deferred closure makes its internal recover() return nil. The handler
	// must run when called as `defer errors.Recover(...)`.
	t.Run("handler is invoked when used as defer errors.Recover", func(t *testing.T) {
		t.Parallel()

		called := false

		func() {
			defer errors.Recover(func(error) {
				called = true
			})

			panic("trigger")
		}()

		assert.True(t, called, "the handler must run for direct defer errors.Recover")
	})

	t.Run("handler does not run when no panic", func(t *testing.T) {
		t.Parallel()

		called := false

		func() {
			defer errors.Recover(func(error) {
				called = true
			})
		}()

		assert.False(t, called)
	})
}

func TestNewFunctionPanicErrorCapturesStack(t *testing.T) {
	t.Parallel()

	err := errors.NewFunctionPanicError("oops")

	assert.True(t, err.IsFunctionPanic())
	assert.Equal(t, "oops", err.Recovered)
	assert.NotEmpty(t, err.Stack)
	assert.Contains(t, err.Error(), "panic in function implementation")
}

type fakeFunctionCallDiagExtra struct {
	functionErr error
}

func (e fakeFunctionCallDiagExtra) CalledFunctionName() string {
	return "run_cmd"
}

func (e fakeFunctionCallDiagExtra) FunctionCallError() error {
	return e.functionErr
}

type functionPanicLikeError struct {
	Recovered any
	Stack     string
}

func (err functionPanicLikeError) Error() string {
	return fmt.Sprintf("panic in function: %v", err.Recovered)
}

func (err functionPanicLikeError) ErrorStack() string {
	return err.Stack
}

func (err functionPanicLikeError) IsFunctionPanic() bool {
	return true
}
