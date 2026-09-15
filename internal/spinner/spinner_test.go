package spinner_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowAfterFastCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		err := spinner.ShowAfter(t.Context(), l, nil, 100*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestShowAfterSlowCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		err := spinner.ShowAfter(t.Context(), l, nil, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "completed",
		}, func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "completed")
	})
}

func TestShowAfterErrorPropagation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		expectedErr := errors.New("test error")

		err := spinner.ShowAfter(t.Context(), l, nil, 100*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			return expectedErr
		})

		require.ErrorIs(t, err, expectedErr)
	})
}

func TestShowAfterSlowError_NoDoneLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		expectedErr := errors.New("test error")

		err := spinner.ShowAfter(t.Context(), l, nil, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			time.Sleep(200 * time.Millisecond)
			return expectedErr
		})

		require.ErrorIs(t, err, expectedErr)
		assert.NotContains(t, buf.String(), "should not appear")
	})
}

func TestShowAfterContextCancelledBeforeTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := spinner.ShowAfter(ctx, l, nil, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestShowAfterContextCancelledAfterTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		ctx, cancel := context.WithCancel(t.Context())

		err := spinner.ShowAfter(ctx, l, spinnerBuf, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			// Wait for spinner to start, then cancel context.
			time.Sleep(150 * time.Millisecond)
			cancel()
			time.Sleep(50 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		// Spinner was shown.
		assert.Contains(t, spinnerBuf.String(), "working...")
		// Done message should NOT appear since context was cancelled.
		assert.NotContains(t, buf.String(), "should not appear")
	})
}

func TestShowAfterSpinnerThenLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := spinner.ShowAfter(
			t.Context(),
			l,
			spinnerBuf,
			50*time.Millisecond,
			spinner.Messages{
				Working: "creating worktree...",
				Done:    "created worktree",
			},
			func() error {
				time.Sleep(500 * time.Millisecond)
				return nil
			},
		)

		require.NoError(t, err)
		// Spinner text was shown during the operation.
		assert.Contains(t, spinnerBuf.String(), "creating worktree...")
		// Done message logged after completion, not the spinner text.
		assert.Contains(t, logBuf.String(), "created worktree")
		assert.NotContains(t, logBuf.String(), "creating worktree...")
	})
}

func TestShowAfterElapsedTimeShown(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := spinner.ShowAfter(t.Context(), l, nil, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			time.Sleep(1200 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		// Elapsed time should be included since it took > 1s.
		assert.Contains(t, logBuf.String(), "finished (")
		assert.Contains(t, logBuf.String(), "s)")
	})
}

func TestShowAfterNonInteractiveKeepalive(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := spinner.ShowAfter(t.Context(), l, nil, 50*time.Millisecond, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			// Sleep long enough for the initial timeout + two keepalive ticks (30s each).
			time.Sleep(61 * time.Second)
			return nil
		})

		require.NoError(t, err)

		output := logBuf.String()
		// Initial spinner message logged.
		assert.Contains(t, output, "working...")
		// Keepalive lines with elapsed time.
		assert.Contains(t, output, "elapsed)")
		// Count keepalive lines — expect at least 2 (at 30s and 60s).
		assert.GreaterOrEqual(t, strings.Count(output, "elapsed)"), 2)
		// Done message logged.
		assert.Contains(t, output, "finished")
	})
}

func TestShowAfterSpinnerNotShownWhenFast(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := spinner.ShowAfter(
			t.Context(),
			l,
			spinnerBuf,
			100*time.Millisecond,
			spinner.Messages{
				Working: "working...",
				Done:    "done",
			},
			func() error {
				return nil
			},
		)

		require.NoError(t, err)
		assert.Empty(t, logBuf.String())
		assert.Empty(t, spinnerBuf.String())
	})
}
