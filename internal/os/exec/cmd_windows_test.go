//go:build windows
// +build windows

package exec_test

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestWindowsConsolePrepare(t *testing.T) {
	t.Parallel()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	testOptions := options.NewTerragruntOptionsWithWriters(&stdout, &stderr)
	testOptions.Logger = log.New(log.WithOutput(&stdout), log.WithLevel(log.DebugLevel))

	exec.PrepareConsole(testOptions.Logger)

	assert.Contains(t, stdout.String(), "msg=\"failed to get console mode: The handle is invalid.")
}

func TestWindowsExitCode(t *testing.T) {
	t.Parallel()

	for i := 0; i <= 255; i++ {
		cmd := exec.Command(`testdata\test_exit_code.bat`, strconv.Itoa(i))
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
	err := errors.New("This is an explicit error")
	retCode, retErr := util.GetExitCode(err)
	assert.Error(t, retErr, "An error was expected")
	assert.Equal(t, err, retErr)
	assert.Equal(t, 0, retCode)
}

func TestWindowsNewSignalsForwarderWait(t *testing.T) {
	t.Parallel()

	expectedWait := 5

	cmd := exec.Command(`testdata\test_sigint_wait.bat`, strconv.Itoa(expectedWait))

	runChannel := make(chan error)

	go func() {
		runChannel <- cmd.Run()
	}()

	time.Sleep(time.Second)
	// start := time.Now()
	// Note: sending interrupt on Windows is not supported by Windows and not implemented in Go
	if cmd.Process != nil { // on some Go versions(Go 1.23, Windows), cmd.Process is nil
		cmd.Process.Signal(os.Kill)
	}

	err := <-runChannel

	assert.Error(t, err)

	// Since we can't send an interrupt on Windows, our test script won't handle it gracefully and exit after the expected wait time,
	// so this part of the test process cannot be done on Windows
	// retCode, err := GetExitCode(err)
	// assert.NoError(t, err)
	// assert.Equal(t, retCode, expectedWait)
	// assert.WithinDuration(t, start.Add(time.Duration(expectedWait)*time.Second), time.Now(), time.Second,
	// 	"Expected to wait 5 (+/-1) seconds after SIGINT")
}
