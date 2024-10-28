//go:build linux || darwin
// +build linux darwin

package exec_test

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errExplicitError = errors.New("this is an explicit error")
)

func TestExitCodeUnix(t *testing.T) {
	t.Parallel()

	for index := 0; index <= 255; index++ {
		cmd := exec.Command("testdata/test_exit_code.sh", strconv.Itoa(index))
		err := cmd.Run()

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

	expectedWait := 5

	cmd := exec.Command("testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(time.Second)
	start := time.Now()
	cmd.Process.Signal(os.Interrupt)

	err := <-runChannel
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)

	assert.Equal(t, expectedWait, retCode)
	assert.WithinDuration(t, time.Now(), start.Add(time.Duration(expectedWait)*time.Second), time.Second,
		"Expected to wait 5 (+/-1) seconds after SIGINT")
}

// There isn't a proper way to catch interrupts in Windows batch scripts, so this test exists only for Unix.
func TestNewSignalsForwarderMultipleUnix(t *testing.T) {
	t.Parallel()

	expectedInterrupts := 10

	cmd := exec.Command("testdata/test_sigint_multiple.sh", strconv.Itoa(expectedInterrupts))

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(time.Second)

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
	require.Error(t, err)

	retCode, err := util.GetExitCode(err)
	require.NoError(t, err)
	assert.LessOrEqual(t, retCode, interrupts, "Subprocess received wrong number of signals")
	assert.Equal(t, expectedInterrupts, retCode, "Subprocess didn't receive multiple signals")
}
