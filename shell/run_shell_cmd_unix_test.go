//go:build linux || darwin
// +build linux darwin

package shell

import (
	"context"
	goerrors "errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestExitCodeUnix(t *testing.T) {
	t.Parallel()

	for i := 0; i <= 255; i++ {
		cmd := exec.Command("../testdata/test_exit_code.sh", strconv.Itoa(i))
		err := cmd.Run()

		if i == 0 {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
		retCode, err := util.GetExitCode(err)
		assert.NoError(t, err)
		assert.Equal(t, i, retCode)
	}

	// assert a non exec.ExitError returns an error
	err := goerrors.New("This is an explicit error")
	retCode, retErr := util.GetExitCode(err)
	assert.Error(t, retErr, "An error was expected")
	assert.Equal(t, err, retErr)
	assert.Equal(t, 0, retCode)
}

func TestNewSignalsForwarderWaitUnix(t *testing.T) {
	t.Parallel()

	expectedWait := 5

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := exec.Command("../testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))

	cmdChannel := make(chan error)
	runChannel := make(chan error)

	signalChannel := NewSignalsForwarder(InterruptSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(1000 * time.Millisecond)
	start := time.Now()
	cmd.Process.Signal(os.Interrupt)
	err = <-runChannel
	cmdChannel <- err
	assert.Error(t, err)
	retCode, err := util.GetExitCode(err)
	assert.NoError(t, err)
	assert.Equal(t, retCode, expectedWait)
	assert.WithinDuration(t, time.Now(), start.Add(time.Duration(expectedWait)*time.Second), time.Second,
		"Expected to wait 5 (+/-1) seconds after SIGINT")

}

// There isn't a proper way to catch interrupts in Windows batch scripts, so this test exists only for Unix
func TestNewSignalsForwarderMultipleUnix(t *testing.T) {
	t.Parallel()

	expectedInterrupts := 10
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := exec.Command("../testdata/test_sigint_multiple.sh", strconv.Itoa(expectedInterrupts))

	cmdChannel := make(chan error)
	runChannel := make(chan error)

	signalChannel := NewSignalsForwarder(InterruptSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(1000 * time.Millisecond)

	interruptAndWaitForProcess := func() (int, error) {
		var interrupts int
		var err error
		for {
			time.Sleep(500 * time.Millisecond)
			select {
			case err = <-runChannel:
				return interrupts, err
			default:
				cmd.Process.Signal(os.Interrupt)
				interrupts++
			}
		}
	}

	interrupts, err := interruptAndWaitForProcess()
	cmdChannel <- err
	assert.Error(t, err)
	retCode, err := util.GetExitCode(err)
	assert.NoError(t, err)
	assert.True(t, retCode <= interrupts, "Subprocess received wrong number of signals")
	assert.Equal(t, retCode, expectedInterrupts, "Subprocess didn't receive multiple signals")
}

func TestRunShellCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	errCh := make(chan error)
	expectedWait := 5

	go func() {
		_, err := RunShellCommandWithOutput(context.Background(), terragruntOptions, "", false, false, "../testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		syscall.Kill(os.Getpid(), syscall.SIGINT)
	})

	expectedErr := fmt.Sprintf("[.] exit status %d", expectedWait)
	assert.EqualError(t, <-errCh, expectedErr)
}
