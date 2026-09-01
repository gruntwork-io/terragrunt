package retry_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errAttempt = errors.New("attempt failed")

// neverAgain declines a second attempt.
func neverAgain(int, error) (time.Duration, bool) { return 0, false }

// alwaysAfter allows another attempt after d.
func alwaysAfter(d time.Duration) func(int, error) (time.Duration, bool) {
	return func(int, error) (time.Duration, bool) { return d, true }
}

func TestDoReturnsOnFirstSuccess(t *testing.T) {
	t.Parallel()

	calls := 0

	err := retry.Do(t.Context(), 3, func(context.Context) error {
		calls++

		return nil
	}, neverAgain)

	require.NoError(t, err)
	assert.Equal(t, 1, calls, "a call that worked is not made again")
}

func TestDoRetriesUntilItSucceeds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		calls := 0
		start := time.Now()

		err := retry.Do(t.Context(), 3, func(context.Context) error {
			calls++
			if calls < 3 {
				return errAttempt
			}

			return nil
		}, alwaysAfter(time.Second))

		require.NoError(t, err)
		assert.Equal(t, 3, calls)
		assert.Equal(t, 2*time.Second, time.Since(start), "one wait before each retry")
	})
}

// TestDoReportsTheLastFailure confirms that a caller sees the error from the final attempt.
func TestDoReportsTheLastFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		last := errors.New("the last one")
		calls := 0

		err := retry.Do(t.Context(), 3, func(context.Context) error {
			calls++
			if calls == 3 {
				return last
			}

			return errAttempt
		}, alwaysAfter(time.Second))

		require.ErrorIs(t, err, last)
		assert.Equal(t, 3, calls, "the attempts are spent")
	})
}

// TestDoStopsWhenNextDeclines confirms that Do stops without waiting when next declines another attempt.
func TestDoStopsWhenNextDeclines(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		calls := 0
		start := time.Now()

		err := retry.Do(t.Context(), 3, func(context.Context) error {
			calls++

			return errAttempt
		}, neverAgain)

		require.ErrorIs(t, err, errAttempt)
		assert.Equal(t, 1, calls)
		assert.Equal(t, time.Duration(0), time.Since(start), "a run that stops does not wait first")
	})
}

// TestDoPassesTheAttemptAndError confirms that next is given the attempt number and the error it decides on.
func TestDoPassesTheAttemptAndError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var attempts []int

		err := retry.Do(t.Context(), 3, func(context.Context) error {
			return errAttempt
		}, func(attempt int, err error) (time.Duration, bool) {
			attempts = append(attempts, attempt)

			assert.ErrorIs(t, err, errAttempt)

			return 0, true
		})

		require.ErrorIs(t, err, errAttempt)
		assert.Equal(t, []int{1, 2}, attempts, "next is not asked after the final attempt")
	})
}

// TestDoStopsWhenTheContextEndsWithRacing confirms that a context cancelled
// during a wait ends the retry with its own error.
func TestDoStopsWhenTheContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go func() {
			synctest.Sleep(time.Second)
			cancel()
		}()

		err := retry.Do(ctx, 3, func(context.Context) error {
			return errAttempt
		}, alwaysAfter(time.Minute))

		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, errAttempt)
	})
}

// TestDoRunsOnceWhenToldOnce confirms that a single-attempt run never consults next.
func TestDoRunsOnceWhenToldOnce(t *testing.T) {
	t.Parallel()

	calls := 0
	asked := false

	err := retry.Do(t.Context(), 1, func(context.Context) error {
		calls++

		return errAttempt
	}, func(int, error) (time.Duration, bool) {
		asked = true

		return 0, true
	})

	require.ErrorIs(t, err, errAttempt)
	assert.Equal(t, 1, calls)
	assert.False(t, asked)
}

// TestDoRefusesFewerThanOneAttempt confirms that Do panics when given fewer than one attempt.
func TestDoRefusesFewerThanOneAttempt(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		_ = retry.Do(t.Context(), 0, func(context.Context) error { return nil }, neverAgain)
	})
}
