//go:build linux || darwin

package exec_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errExplicitError = errors.New("this is an explicit error")
)

func TestExitCodeUnix(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	for index := 0; index <= 255; index++ {
		cmd := exec.Command(t.Context(), vexec.NewOSExec(), "testdata/test_exit_code.sh", strconv.Itoa(index))
		err := cmd.Run(l)

		if index == 0 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}

		retCode, err := util.GetExitCode(err)
		require.NoError(t, err)
		assert.Equal(t, index, retCode)
	}

	// assert a non exec.ExitError returns an error
	retCode, retErr := util.GetExitCode(errExplicitError)
	require.Error(t, retErr, "An error was expected")
	assert.Equal(t, errExplicitError, retErr)
	assert.Equal(t, 0, retCode)
}

func TestNewSignalsForwarderWaitUnix(t *testing.T) {
	t.Parallel()

	expectedWait := 1

	l := logger.CreateLogger()

	cmd := exec.Command(t.Context(), vexec.NewOSExec(), "testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))
	ready := make(chan struct{})
	cmd.SetStdout(&signalReadyWriter{ready: ready})

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	waitForSignalTestReady(t, ready)

	start := time.Now()

	cmd.SendSignal(l, os.Interrupt)

	err := <-runChannel
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)

	assert.Equal(t, expectedWait, retCode)
	assert.WithinDuration(t, time.Now(), start.Add(time.Duration(expectedWait)*time.Second), time.Second,
		"Expected to wait %d (+/-1) seconds after SIGINT", expectedWait)
}

// There isn't a proper way to catch interrupts in Windows batch scripts, so this test exists only for Unix.
func TestNewSignalsForwarderMultipleUnix(t *testing.T) {
	t.Parallel()

	expectedInterrupts := 3

	l := logger.CreateLogger()

	cmd := exec.Command(
		t.Context(), vexec.NewOSExec(),
		"testdata/test_sigint_multiple.sh", strconv.Itoa(expectedInterrupts),
	)
	ready := make(chan struct{})
	cmd.SetStdout(&signalReadyWriter{ready: ready})

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	waitForSignalTestReady(t, ready)

	interruptAndWaitForProcess := func() (int, error) {
		var (
			interrupts int
			err        error
		)

		for {
			time.Sleep(500 * time.Millisecond)

			select {
			case err = <-runChannel:
				return interrupts, err
			default:
				cmd.SendSignal(l, os.Interrupt)

				interrupts++
			}
		}
	}

	interrupts, err := interruptAndWaitForProcess()
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)
	assert.LessOrEqual(t, retCode, interrupts, "Subprocess received wrong number of signals")
	assert.Equal(t, expectedInterrupts, retCode, "Subprocess didn't receive multiple signals")
}

// TestGracefulShutdownOnContextCancelUnix verifies that when the context is cancelled
// without a signal cause, the Cancel callback sends SIGINT (not SIGKILL) to allow
// processes like Terraform to gracefully shutdown their child processes.
// The test script traps SIGINT and exits with code 42, while SIGKILL would terminate
// it immediately without running the trap handler.
func TestGracefulShutdownOnContextCancelUnix(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	l := logger.CreateLogger()

	cmd := exec.Command(ctx, vexec.NewOSExec(), "testdata/test_graceful_shutdown.sh")
	ready := make(chan struct{})
	cmd.SetStdout(&signalReadyWriter{ready: ready})

	cmd.Configure(exec.WithGracefulShutdownDelay(5 * time.Second))

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	waitForSignalTestReady(t, ready)

	cancel()

	err := <-runChannel
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)

	assert.Equal(
		t,
		42,
		retCode,
		"Expected exit code 42 (SIGINT received), but got %d. "+
			"This suggests SIGKILL was sent instead of SIGINT.",
		retCode,
	)
}

type signalReadyWriter struct {
	ready chan<- struct{}
	once  sync.Once
}

func (writer *signalReadyWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("ready")) {
		writer.once.Do(func() {
			close(writer.ready)
		})
	}

	return len(data), nil
}

func waitForSignalTestReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()

	select {
	case <-ready:
		return
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for signal test process readiness")
	}
}
