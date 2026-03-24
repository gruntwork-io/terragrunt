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

	err := util.NotifyIfSlow(context.Background(), l, 100*time.Millisecond, "should not appear", func() error {
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

	err := util.NotifyIfSlow(context.Background(), l, 50*time.Millisecond, "operation is slow", func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "operation is slow")
}

func TestNotifyIfSlow_ErrorPropagation(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	expectedErr := errors.New("test error")

	err := util.NotifyIfSlow(context.Background(), l, 100*time.Millisecond, "should not appear", func() error {
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

	err := util.NotifyIfSlow(ctx, l, 50*time.Millisecond, "should not appear", func() error {
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

	val, err := util.NotifyIfSlowV(context.Background(), l, 100*time.Millisecond, "should not appear", func() (string, error) {
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

	val, err := util.NotifyIfSlowV(context.Background(), l, 50*time.Millisecond, "value op is slow", func() (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 42, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 42, val)
	assert.Contains(t, buf.String(), "value op is slow")
}
