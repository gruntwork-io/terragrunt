package util_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyIfSlow_FastCompletion(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	err := util.NotifyIfSlow(context.Background(), l, nil, 100*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "done",
	}, func() error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestNotifyIfSlow_SlowCompletion(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	err := util.NotifyIfSlow(context.Background(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "completed",
	}, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "completed")
}

func TestNotifyIfSlow_ErrorPropagation(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	expectedErr := errors.New("test error")

	err := util.NotifyIfSlow(context.Background(), l, nil, 100*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "done",
	}, func() error {
		return expectedErr
	})

	require.ErrorIs(t, err, expectedErr)
}

func TestNotifyIfSlow_ContextCancelled(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := util.NotifyIfSlow(ctx, l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "done",
	}, func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestNotifyIfSlowV_ReturnsValue(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	val, err := util.NotifyIfSlowV(context.Background(), l, nil, 100*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "done",
	}, func() (string, error) {
		return "result", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "result", val)
	assert.Empty(t, buf.String())
}

func TestNotifyIfSlowV_SlowWithValue(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	val, err := util.NotifyIfSlowV(context.Background(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "computing...",
		Done:    "computed",
	}, func() (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 42, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 42, val)
	assert.Contains(t, buf.String(), "computed")
}

func TestNotifyIfSlow_SpinnerThenLog(t *testing.T) {
	t.Parallel()

	logBuf := new(bytes.Buffer)
	spinnerBuf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

	err := util.NotifyIfSlow(context.Background(), l, spinnerBuf, 50*time.Millisecond, util.SlowNotifyMsg{
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
}

func TestNotifyIfSlow_ElapsedTimeShown(t *testing.T) {
	t.Parallel()

	logBuf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

	err := util.NotifyIfSlow(context.Background(), l, nil, 50*time.Millisecond, util.SlowNotifyMsg{
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
}

func TestNotifyIfSlow_SpinnerNotShownWhenFast(t *testing.T) {
	t.Parallel()

	logBuf := new(bytes.Buffer)
	spinnerBuf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(logBuf))

	err := util.NotifyIfSlow(context.Background(), l, spinnerBuf, 100*time.Millisecond, util.SlowNotifyMsg{
		Spinner: "working...",
		Done:    "done",
	}, func() error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, logBuf.String())
	assert.Empty(t, spinnerBuf.String())
}
