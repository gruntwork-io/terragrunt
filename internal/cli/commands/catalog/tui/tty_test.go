package tui_test

import (
	"errors"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingCloser counts Close calls so tests can assert the TTY probe
// releases its handles.
type recordingCloser struct {
	closes int
}

func (c *recordingCloser) Close() error {
	c.closes++

	return nil
}

// TestEnsureTTY_TerminalStdinSkipsProbe verifies that a terminal stdin is
// accepted without opening the controlling terminal.
func TestEnsureTTY_TerminalStdinSkipsProbe(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	probed := false
	openTTY := func() (io.Closer, io.Closer, error) {
		probed = true

		return nil, nil, errors.New("must not be called")
	}

	err := tui.EnsureTTY(l, func() bool { return true }, openTTY)

	require.NoError(t, err)
	assert.False(t, probed, "openTTY must not be probed when stdin is already a terminal")
}

// TestEnsureTTY_ProbeSuccessReleasesHandles verifies that a successful
// controlling-terminal probe closes both handles.
func TestEnsureTTY_ProbeSuccessReleasesHandles(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	in := &recordingCloser{}
	out := &recordingCloser{}
	openTTY := func() (io.Closer, io.Closer, error) {
		return in, out, nil
	}

	err := tui.EnsureTTY(l, func() bool { return false }, openTTY)

	require.NoError(t, err)
	assert.Equal(t, 1, in.closes, "probe input handle should be closed exactly once")
	assert.Equal(t, 1, out.closes, "probe output handle should be closed exactly once")
}

// TestEnsureTTY_SharedHandleClosedOnce verifies the POSIX case where the
// probe returns the same /dev/tty handle for input and output: it must be
// closed only once.
func TestEnsureTTY_SharedHandleClosedOnce(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	shared := &recordingCloser{}
	openTTY := func() (io.Closer, io.Closer, error) {
		return shared, shared, nil
	}

	err := tui.EnsureTTY(l, func() bool { return false }, openTTY)

	require.NoError(t, err)
	assert.Equal(t, 1, shared.closes, "shared probe handle should be closed exactly once")
}

// TestEnsureTTY_NoTerminal verifies that a failed probe surfaces the typed
// ErrNoTerminal while preserving the underlying cause.
func TestEnsureTTY_NoTerminal(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	cause := errors.New("no controlling terminal")
	openTTY := func() (io.Closer, io.Closer, error) {
		return nil, nil, cause
	}

	err := tui.EnsureTTY(l, func() bool { return false }, openTTY)

	require.Error(t, err)
	require.ErrorIs(t, err, tui.ErrNoTerminal)
	require.ErrorIs(t, err, cause, "the probe failure should stay in the error chain")
}
