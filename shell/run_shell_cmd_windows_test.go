//go:build windows
// +build windows

package shell

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestWindowsRunShellCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	errCh := make(chan error)
	expectedWait := 5

	ctx, cancel := context.WithCancelCause(context.Background())

	cmdPath := "testdata\\test_sigint_wait.bat"

	go func() {
		_, err := RunShellCommandWithOutput(ctx, terragruntOptions, "", false, false, cmdPath, strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		cancel(signal.NewContextCanceledError(os.Kill))
	})

	actualErr := <-errCh
	assert.Contains(t, actualErr.Error(), fmt.Sprintf("Failed to execute \"%s", cmdPath))
	assert.Contains(t, actualErr.Error(), fmt.Sprintf("exit status %d", expectedWait))
}
