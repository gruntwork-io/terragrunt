//go:build linux || darwin
// +build linux darwin

package shell_test

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/require"
)

func TestRunCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	l := logger.CreateLogger()

	errCh := make(chan error)
	expectedWait := 5

	ctx, cancel := context.WithCancelCause(t.Context())

	cmdPath := "testdata/test_sigint_wait.sh"

	go func() {
		_, err := shell.RunCommandWithOutput(ctx, l, terragruntOptions, "", false, false, cmdPath, strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		cancel(signal.NewContextCanceledError(syscall.SIGINT))
	})

	actualErr := <-errCh
	require.Error(t, actualErr, "Expected an error but got none")

	// The process might either exit with the expected status code or be killed by a signal
	// depending on timing and system conditions
	expectedExitStatusErr := fmt.Sprintf("Failed to execute \"%s 5\" in .\n\nexit status %d", cmdPath, expectedWait)
	expectedKilledErr := fmt.Sprintf("Failed to execute \"%s 5\" in .\n\nsignal: killed", cmdPath)

	if actualErr.Error() != expectedExitStatusErr && actualErr.Error() != expectedKilledErr {
		t.Errorf("Expected error to be either:\n  %s\nor:\n  %s\nbut got:\n  %s",
			expectedExitStatusErr, expectedKilledErr, actualErr.Error())
	}
}
