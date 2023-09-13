//go:build windows
// +build windows

package shell

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestWindowsConsolePrepare(t *testing.T) {
	t.Parallel()

	testOptions := options.NewTerragruntOptions()

	stdout := bytes.Buffer{}

	var testLogger = logrus.New()
	testLogger.Out = &stdout
	testLogger.Level = logrus.DebugLevel

	testOptions.Logger = testLogger.WithContext(context.Background())

	PrepareConsole(testOptions)

	assert.Contains(t, stdout.String(), "level=debug msg=\"failed to get console mode: The handle is invalid.")
}

func TestExitCodeWindows(t *testing.T) {
	t.Parallel()

	for i := 0; i <= 255; i++ {
		cmd := exec.Command(`..\testdata\test_exit_code.bat`, strconv.Itoa(i))
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

func TestNewSignalsForwarderWaitWindows(t *testing.T) {
	t.Parallel()

	expectedWait := 5

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := exec.Command(`..\testdata\test_sigint_wait.bat`, strconv.Itoa(expectedWait))

	cmdChannel := make(chan error)
	runChannel := make(chan error)

	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(1000 * time.Millisecond)
	// start := time.Now()
	// Note: sending interrupt on Windows is not supported by Windows and not implemented in Go
	cmd.Process.Signal(os.Kill)
	err = <-runChannel
	cmdChannel <- err
	assert.Error(t, err)

	// Since we can't send an interrupt on Windows, our test script won't handle it gracefully and exit after the expected wait time,
	// so this part of the test process cannot be done on Windows
	// retCode, err := GetExitCode(err)
	// assert.Nil(t, err)
	// assert.Equal(t, retCode, expectedWait)
	// assert.WithinDuration(t, start.Add(time.Duration(expectedWait)*time.Second), time.Now(), time.Second,
	// 	"Expected to wait 5 (+/-1) seconds after SIGINT")
}

func TestRunShellCommandWithOutputInterrupt(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	errCh := make(chan error)
	expectedWait := 5

	go func() {
		_, err := RunShellCommandWithOutput(terragruntOptions, "", false, false, "../testdata/test_sigint_wait.bat", strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		process, err := os.FindProcess(os.Getpid())
		assert.NoError(t, err)

		process.Signal(os.Kill)
	})

	expectedErr := fmt.Sprintf("[.] exit status %d", expectedWait)
	assert.EqualError(t, <-errCh, expectedErr)
}
