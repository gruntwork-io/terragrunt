package shell

import (
	goerrors "errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions := options.NewTerragruntOptionsForTest("")
	cmd := RunShellCommand(terragruntOptions, "/bin/bash", "-c", "true")
	assert.Nil(t, cmd)

	cmd = RunShellCommand(terragruntOptions, "/bin/bash", "-c", "false")
	assert.Error(t, cmd)
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	for i := 0; i <= 255; i++ {
		cmd := exec.Command("../testdata/test_exit_code.sh", strconv.Itoa(i))
		err := cmd.Run()

		if i == 0 {
			assert.Nil(t, err)
		} else {
			assert.Error(t, err)
		}
		retCode, err := GetExitCode(err)
		assert.Nil(t, err)
		assert.Equal(t, i, retCode)
	}

	// assert a non exec.ExitError returns an error
	err := goerrors.New("This is an explicit error")
	retCode, retErr := GetExitCode(err)
	assert.Error(t, retErr, "An error was expected")
	assert.Equal(t, err, retErr)
	assert.Equal(t, 0, retCode)
}

func TestNewSignalsForwarderWait(t *testing.T) {
	t.Parallel()

	expectedWait := 5

	terragruntOptions := options.NewTerragruntOptionsForTest("")
	cmd := exec.Command("../testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))

	cmdChannel := make(chan error)
	runChannel := make(chan error)

	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(1000 * time.Millisecond)
	start := time.Now()
	cmd.Process.Signal(os.Interrupt)
	err := <-runChannel
	cmdChannel <- err
	assert.Error(t, err)
	retCode, err := GetExitCode(err)
	assert.Nil(t, err)
	assert.Equal(t, retCode, expectedWait)
	assert.WithinDuration(t, time.Now(), start.Add(time.Duration(expectedWait)*time.Second), time.Second,
		"Expected to wait 5 (+/-1) seconds after SIGINT")

}

func TestNewSignalsForwarderMultiple(t *testing.T) {
	t.Parallel()

	expectedInterrupts := 10
	terragruntOptions := options.NewTerragruntOptionsForTest("")
	cmd := exec.Command("../testdata/test_sigint_multiple.sh", strconv.Itoa(expectedInterrupts))

	cmdChannel := make(chan error)
	runChannel := make(chan error)

	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
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
	retCode, err := GetExitCode(err)
	assert.Nil(t, err)
	assert.Equal(t, retCode, interrupts)

}
