//go:build linux || darwin

package shell_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

func TestRunCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	terragruntOptions.Writers = writer.Writers{
		Writer:    &shellSignalReadyWriter{ready: ready},
		ErrWriter: io.Discard,
	}

	l := logger.CreateLogger()

	errCh := make(chan error)
	expectedWait := 1

	ctx, cancel := context.WithCancelCause(t.Context())

	cmdPath := "testdata/test_sigint_wait.sh"

	go func() {
		runOpts := configbridge.ShellRunOptsFromOpts(terragruntOptions).WithForwardSignalDelay(10 * time.Millisecond)

		_, err := shell.RunCommandWithOutput(ctx, l, vexec.NewOSExec(), runOpts, "", false, false, cmdPath, strconv.Itoa(expectedWait))
		errCh <- err
	}()

	waitForShellSignalTestReady(t, ready)
	cancel(signal.NewContextCanceledError(syscall.SIGINT))

	actualErr := <-errCh
	require.Error(t, actualErr, "Expected an error but got none")

	// The process might either exit with the expected status code or be killed by a signal
	// depending on timing and system conditions
	expectedExitStatusErr := fmt.Sprintf("Failed to execute \"%s %d\" in .\n\nexit status %d", cmdPath, expectedWait, expectedWait)
	expectedKilledErr := fmt.Sprintf("Failed to execute \"%s %d\" in .\n\nsignal: killed", cmdPath, expectedWait)

	if actualErr.Error() == expectedKilledErr {
		t.Errorf("Expected process to gracefully terminate but got\n: %s", actualErr.Error())

		return
	}

	if actualErr.Error() != expectedExitStatusErr {
		t.Errorf("Expected error to be:\n  %s\nbut got:\n  %s", expectedExitStatusErr, actualErr.Error())
	}
}

type shellSignalReadyWriter struct {
	ready chan<- struct{}
	once  sync.Once
}

func (writer *shellSignalReadyWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("ready")) {
		writer.once.Do(func() {
			close(writer.ready)
		})
	}

	return len(data), nil
}

func waitForShellSignalTestReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()

	select {
	case <-ready:
		return
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for shell signal test process readiness")
	}
}
