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

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunShellCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	errCh := make(chan error)
	expectedWait := 5

	ctx, cancel := context.WithCancelCause(context.Background())

	cmdPath := "testdata/test_sigint_wait.sh"

	go func() {
		_, err := shell.RunShellCommandWithOutput(ctx, terragruntOptions, "", false, false, cmdPath, strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		cancel(signal.NewContextCanceledError(syscall.SIGINT))
	})

	actualErr := <-errCh
	expectedErr := fmt.Sprintf("Failed to execute \"%s 5\" in .\n\nexit status %d", cmdPath, expectedWait)
	assert.EqualError(t, actualErr, expectedErr)
}
