package util_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyIfSlow_FastCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		err := util.NotifyIfSlow(t.Context(), l, nil, 100*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "done",
		}, func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestNotifyIfSlow_SlowCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		err := util.NotifyIfSlow(t.Context(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "completed",
		}, func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "completed")
	})
}

func TestNotifyIfSlow_ErrorPropagation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		expectedErr := errors.New("test error")

		err := util.NotifyIfSlow(t.Context(), l, nil, 100*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "done",
		}, func() error {
			return expectedErr
		})

		require.ErrorIs(t, err, expectedErr)
	})
}

func TestNotifyIfSlow_SlowError_NoDoneLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		expectedErr := errors.New("test error")

		err := util.NotifyIfSlow(t.Context(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "should not appear",
		}, func() error {
			time.Sleep(200 * time.Millisecond)
			return expectedErr
		})

		require.ErrorIs(t, err, expectedErr)
		assert.NotContains(t, buf.String(), "should not appear")
	})
}

func TestNotifyIfSlow_ContextCancelledBeforeTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := util.NotifyIfSlow(ctx, l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "should not appear",
		}, func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestNotifyIfSlow_ContextCancelledAfterTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

		ctx, cancel := context.WithCancel(t.Context())

		err := util.NotifyIfSlow(ctx, l, spinnerBuf, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
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

func TestNotifyIfSlow_SpinnerThenLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := util.NotifyIfSlow(t.Context(), l, spinnerBuf, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "creating worktree...",
			Done:    "created worktree",
		}, func() error {
			time.Sleep(500 * time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		// Spinner text was shown during the operation.
		assert.Contains(t, spinnerBuf.String(), "creating worktree...")
		// Done message logged after completion, not the spinner text.
		assert.Contains(t, logBuf.String(), "created worktree")
		assert.NotContains(t, logBuf.String(), "creating worktree...")
	})
}

func TestNotifyIfSlow_ElapsedTimeShown(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := util.NotifyIfSlow(t.Context(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
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

func TestNotifyIfSlow_NonInteractiveKeepalive(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := util.NotifyIfSlow(t.Context(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
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

func TestNotifyIfSlow_SpinnerNotShownWhenFast(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logBuf := new(bytes.Buffer)
		spinnerBuf := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

		err := util.NotifyIfSlow(t.Context(), l, spinnerBuf, 100*time.Millisecond, util.SlowNotifyMsg{
			Spinner: "working...",
			Done:    "done",
		}, func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, logBuf.String())
		assert.Empty(t, spinnerBuf.String())
	})
}
