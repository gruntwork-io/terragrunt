//go:build windows
// +build windows

package shell

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const InvalidHandleErrorMessage = "The handle is invalid"

// PrepareConsole enables support for escape sequences
// https://stackoverflow.com/questions/56460651/golang-fmt-print-033c-and-fmt-print-x1bc-are-not-clearing-screenansi-es
// https://github.com/containerd/console/blob/f652dc3/console_windows.go#L46
func PrepareConsole(terragruntOptions *options.TerragruntOptions) {
	enableVirtualTerminalProcessing(terragruntOptions, os.Stdin)
	enableVirtualTerminalProcessing(terragruntOptions, os.Stderr)
	enableVirtualTerminalProcessing(terragruntOptions, os.Stdout)
}

func enableVirtualTerminalProcessing(options *options.TerragruntOptions, file *os.File) {
	var mode uint32
	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		if strings.Contains(err.Error(), InvalidHandleErrorMessage) {
			options.Logger.Debugf("failed to get console mode: %v\n", err)
		} else {
			options.Logger.Errorf("failed to get console mode: %v\n", err)
		}
		return
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		options.Logger.Errorf("failed to set console mode: %v\n", err)
		if secondError := windows.SetConsoleMode(handle, mode); secondError != nil {
			options.Logger.Errorf("failed to set console mode: %v\n", secondError)
			return
		}
	}
}

// For windows, there is no concept of a pseudoTTY so we run as if there is no pseudoTTY.
func runCommandWithPTTY(terragruntOptions *options.TerragruntOptions, cmd *exec.Cmd, cmdStdout io.Writer, cmdStderr io.Writer) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = cmdStdout
	cmd.Stderr = cmdStderr
	if err := cmd.Start(); err != nil {
		// bad path, binary not executable, &c
		return errors.WithStackTrace(err)
	}
	return nil
}
