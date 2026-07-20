//go:build linux || darwin

package shell_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	l := logger.CreateLogger()

	errCh := make(chan error)
	expectedWait := 1

	ctx, cancel := context.WithCancelCause(t.Context())

	cmdPath := "testdata/test_sigint_wait.sh"
	readyPath := filepath.Join(t.TempDir(), "sigint-ready")

	shellOpts := configbridge.ShellRunOptsFromOpts(terragruntOptions)
	// Forward the interrupt near-immediately instead of waiting the production
	// grace period so the test does not pay that delay.
	shellOpts.SignalForwardingDelay = 100 * time.Millisecond

	go func() {
		_, err := shell.RunCommandWithOutput(
			ctx,
			l,
			venv.OSVenv(),
			shellOpts,
			"",
			false,
			false,
			cmdPath,
			strconv.Itoa(expectedWait),
			readyPath,
		)
		errCh <- err
	}()

	// Cancel only once the child confirms its INT trap is installed, instead of guessing with a timer.
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyPath)

		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "child never wrote the trap-ready marker")

	cancel(signal.NewContextCanceledError(syscall.SIGINT))

	actualErr := <-errCh
	require.Error(t, actualErr, "Expected an error but got none")

	// A graceful shutdown lets the script's SIGINT trap run and exit with
	// expectedWait; a SIGKILL would report a signal status instead.
	retCode, codeErr := util.GetExitCode(actualErr)
	require.NoError(t, codeErr)
	assert.Equal(t, expectedWait, retCode,
		"expected the subprocess to gracefully handle SIGINT and exit with %d", expectedWait)
}
