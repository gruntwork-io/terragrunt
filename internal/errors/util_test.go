package errors_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty/function"
)

type fakeFunctionCallDiagExtra struct {
	functionErr error
}

func (e fakeFunctionCallDiagExtra) CalledFunctionName() string {
	return "run_cmd"
}

func (e fakeFunctionCallDiagExtra) FunctionCallError() error {
	return e.functionErr
}

func TestIsFunctionPanic(t *testing.T) {
	t.Parallel()

	t.Run("custom function panic marker", func(t *testing.T) {
		t.Parallel()

		err := errors.FunctionPanicError{
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

	t.Run("non panic error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, errors.IsFunctionPanic(assert.AnError))
	})
}

func TestErrorStackForFunctionPanic(t *testing.T) {
	t.Parallel()

	t.Run("includes function panic stack", func(t *testing.T) {
		t.Parallel()

		err := errors.FunctionPanicError{
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
