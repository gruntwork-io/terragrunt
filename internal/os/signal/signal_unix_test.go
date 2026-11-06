//go:build !windows

package signal_test

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalWait bounds how long a test waits for a signal it sent to itself.
const signalWait = 30 * time.Second

func TestNotifier_DeliversSignal(t *testing.T) {
	t.Parallel()

	got := make(chan os.Signal, 1)

	signal.Notifier(forward(got), syscall.SIGUSR1)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGUSR1))

	assert.Equal(t, syscall.SIGUSR1, waitForSignal(t, got))
}

func TestNotifierWithContext_DeliversSignal(t *testing.T) {
	t.Parallel()

	got := make(chan os.Signal, 1)

	signal.NotifierWithContext(t.Context(), forward(got), syscall.SIGUSR2)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGUSR2))

	assert.Equal(t, syscall.SIGUSR2, waitForSignal(t, got))
}

func waitForSignal(t *testing.T, got <-chan os.Signal) os.Signal {
	t.Helper()

	select {
	case sig := <-got:
		return sig
	case <-time.After(signalWait):
		require.FailNow(t, "signal was not delivered")
	}

	return nil
}

// forward returns a callback that hands each signal to got without blocking.
func forward(got chan<- os.Signal) signal.NotifyFunc {
	return func(sig os.Signal) {
		select {
		case got <- sig:
		default:
		}
	}
}
