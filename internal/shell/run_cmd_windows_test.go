//go:build windows

package shell_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsRunCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest")

	l := logger.CreateLogger()
	v := venv.OSVenv()

	errCh := make(chan error)
	expectedWait := 5

	ctx, cancel := context.WithCancelCause(t.Context())

	cmdPath := "testdata\\test_sigint_wait.bat"

	go func() {
		_, err := shell.RunCommandWithOutput(
			ctx,
			l,
			v,
			configbridge.ShellRunOptsFromOpts(v.Env, terragruntOptions),
			"",
			false,
			false,
			cmdPath,
			strconv.Itoa(expectedWait),
		)
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		cancel(signal.NewContextCanceledError(os.Kill))
	})

	actualErr := <-errCh
	require.Error(t, actualErr, "Expected an error but got none")

	// The process might either exit with the expected status code or be killed by a signal
	// depending on timing and system conditions. On Windows, the error message might also
	// include stderr output from the batch file execution.

	// Check if the error contains the expected patterns rather than exact matches
	// since Windows batch files might include additional stderr output
	actualErrStr := actualErr.Error()
	containsExitStatus5 := strings.Contains(actualErrStr, "exit status 5")
	containsExitStatus1 := strings.Contains(actualErrStr, "exit status 1")
	containsFailedExecute := strings.Contains(
		actualErrStr,
		"Failed to execute \""+cmdPath,
	)

	assert.NotContains(t, actualErrStr, "signal: killed", "Expected process to gracefully terminate")

	// On Windows, the batch file might exit with status 1 when interrupted, or be killed by signal
	assert.True(
		t,
		containsFailedExecute && (containsExitStatus5 || containsExitStatus1),
		"Expected error to contain 'Failed to execute \"%s' and either 'exit status 5', or 'exit status 1', but got:\n  %s",
		cmdPath,
		actualErrStr,
	)
}
