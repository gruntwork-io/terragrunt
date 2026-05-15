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
	})

	t.Run("generic function panic by error string and stack", func(t *testing.T) {
		t.Parallel()

		err := functionPanicLikeError{
			Recovered: "slice bounds out of range",
			Stack:     "generic panic stack",
		}

		assert.True(t, errors.IsFunctionPanic(err))
		assert.Equal(t, "generic panic stack", errors.ErrorStack(err))
	})

	t.Run("typed panic-shaped error with marker message", func(t *testing.T) {
		t.Parallel()

		err := functionPanicLikeError{
			Recovered: "slice bounds out of range",
			Stack:     "typed panic stack",
		}

		assert.True(t, errors.IsFunctionPanic(err))
		assert.Equal(t, "typed panic stack", errors.ErrorStack(err))
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

	t.Run("wrapped error message that contains panic", func(t *testing.T) {
		t.Parallel()

		err := errors.New("panic: function call failed: runtime error")

		assert.True(t, errors.IsFunctionPanic(err))
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
		assert.Contains(t, recovered.Error(), "panic:")
		assert.NotEmpty(t, errors.ErrorStack(recovered))
	})

	t.Run("preserves function panic error from recover", func(t *testing.T) {
		t.Parallel()

		panicErr := functionPanicLikeError{
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
		assert.NotEmpty(t, errors.ErrorStack(recovered))
	})

	t.Run("preserves function panic-like shape from recover", func(t *testing.T) {
		t.Parallel()

		recoveredErr := functionPanicLikeError{
			Recovered: "runtime error",
			Stack:     "typed recover stack",
		}

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic(recoveredErr)
		}()

		assert.True(t, errors.IsFunctionPanic(recovered))
		assert.NotEmpty(t, errors.ErrorStack(recovered))
	})
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

func (err functionPanicLikeError) IsFunctionPanic() bool {
	return true
}
