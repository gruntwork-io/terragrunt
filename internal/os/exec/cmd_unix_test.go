//go:build linux || darwin

package exec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// requireTrapReady blocks until the subprocess writes its marker file. Signalling on a
// timer instead races the child's startup: a SIGINT that lands before the script reaches
// its `trap` line kills it outright, so the handler never runs and the exit code is the
// signal status rather than the value the test asserts on.
func requireTrapReady(t *testing.T, readyPath string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := os.Stat(readyPath)

		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "child never wrote the trap-ready marker")
}

// requireInterruptCount blocks until the subprocess records that its INT trap has run
// want times.
func requireInterruptCount(t *testing.T, readyPath string, want int) {
	t.Helper()

	require.Eventually(t, func() bool {
		content, err := os.ReadFile(readyPath)
		if err != nil {
			return false
		}

		got, err := strconv.Atoi(strings.TrimSpace(string(content)))

		return err == nil && got == want
	}, 10*time.Second, 10*time.Millisecond, "child never acknowledged interrupt %d", want)
}

func TestExitCodeUnix(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	for index := 0; index <= 255; index++ {
		cmd := exec.Command(
			t.Context(),
			vexec.NewOSExec(),
			"testdata/test_exit_code.sh",
			strconv.Itoa(index),
		)
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

	expectedWait := 2

	l := logger.CreateLogger()

	readyPath := filepath.Join(t.TempDir(), "sigint-ready")

	cmd := exec.Command(
		t.Context(),
		vexec.NewOSExec(),
		"testdata/test_sigint_wait.sh",
		strconv.Itoa(expectedWait),
		readyPath,
	)

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	requireTrapReady(t, readyPath)

	start := time.Now()

	cmd.SendSignal(l, os.Interrupt)

	err := <-runChannel
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)

	assert.Equal(t, expectedWait, retCode)
	assert.WithinDuration(
		t,
		time.Now(),
		start.Add(time.Duration(expectedWait)*time.Second),
		time.Second,
		"Expected to wait 5 (+/-1) seconds after SIGINT",
	)
}

// There isn't a proper way to catch interrupts in Windows batch scripts, so this test exists only for Unix.
func TestNewSignalsForwarderMultipleUnix(t *testing.T) {
	t.Parallel()

	expectedInterrupts := 4

	l := logger.CreateLogger()

	readyPath := filepath.Join(t.TempDir(), "sigint-ready")

	cmd := exec.Command(
		t.Context(), vexec.NewOSExec(),
		"testdata/test_sigint_multiple.sh", strconv.Itoa(expectedInterrupts), readyPath,
	)

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	requireTrapReady(t, readyPath)

	// Bash defers its trap until the running `sleep` returns, so two signals delivered within
	// one sleep window collapse into a single handler run. Waiting for the child to
	// acknowledge each interrupt before sending the next keeps the count exact.
	for interrupts := 1; interrupts <= expectedInterrupts; interrupts++ {
		cmd.SendSignal(l, os.Interrupt)

		requireInterruptCount(t, readyPath, interrupts)
	}

	err := <-runChannel
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)
	assert.Equal(t, expectedInterrupts, retCode, "Subprocess didn't receive multiple signals")
}

// TestGracefulShutdownOnContextCancelUnix verifies that when the context is cancelled
// without a signal cause, the Cancel callback sends SIGINT (not SIGKILL) to allow
// processes like Terraform to gracefully shutdown their child processes.
// The test script traps SIGINT and exits with code 42, while SIGKILL would terminate
// it immediately without running the trap handler.
func TestGracefulShutdownOnContextCancelUnix(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	l := logger.CreateLogger()

	readyPath := filepath.Join(t.TempDir(), "sigint-ready")

	cmd := exec.Command(ctx, vexec.NewOSExec(), "testdata/test_graceful_shutdown.sh", readyPath)

	cmd.Configure(exec.WithGracefulShutdownDelay(5 * time.Second))

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run(l)
	}()

	requireTrapReady(t, readyPath)

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
