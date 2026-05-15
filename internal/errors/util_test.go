package errors_test

import (
	stdErrors "errors"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorStack(t *testing.T) {
	t.Parallel()

	t.Run("returns empty for plain error without stack", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, errors.ErrorStack(stdErrors.New("plain")))
	})

	t.Run("returns the stack from a go-errors wrapped error", func(t *testing.T) {
		t.Parallel()

		stack := errors.ErrorStack(errors.New("wrapped"))
		require.NotEmpty(t, stack)
		assert.Contains(t, stack, "TestErrorStack")
	})

	// Regression: ErrorStack used to invoke a marker-interface branch and
	// a reflection-based field walker, appending the same stack twice for
	// any go-errors wrapped error.
	t.Run("does not duplicate frames for wrapped errors", func(t *testing.T) {
		t.Parallel()

		stack := errors.ErrorStack(errors.New("wrapped"))

		needle := "TestErrorStack"
		assert.LessOrEqual(t, strings.Count(stack, needle), 2, "ErrorStack must not duplicate frames")
	})
}

func TestRecoverWrapsPanic(t *testing.T) {
	t.Parallel()

	t.Run("recovers non-error panic and tags message with panic prefix", func(t *testing.T) {
		t.Parallel()

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic("panic-value")
		}()

		require.Error(t, recovered)
		assert.Contains(t, recovered.Error(), "panic:")
		assert.Contains(t, recovered.Error(), "panic-value")
		assert.Contains(t, recovered.Error(), "runtime/panic.go")
	})

	t.Run("preserves wrapped error chain for error-typed panic", func(t *testing.T) {
		t.Parallel()

		original := stdErrors.New("boom")

		var recovered error

		func() {
			defer errors.Recover(func(err error) {
				recovered = err
			})

			panic(original)
		}()

		require.Error(t, recovered)
		require.ErrorIs(t, recovered, original, "recovered error must wrap the original via %%w")
		assert.Contains(t, recovered.Error(), "panic:")
		assert.Contains(t, recovered.Error(), "runtime/panic.go")
	})

	// Regression: invoking errors.Recover indirectly through another
	// deferred closure makes its internal recover() return nil. The handler
	// must run when called as `defer errors.Recover(...)`.
	t.Run("handler runs only when used as defer errors.Recover", func(t *testing.T) {
		t.Parallel()

		called := false

		func() {
			defer errors.Recover(func(error) {
				called = true
			})

			panic("trigger")
		}()

		assert.True(t, called)
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
