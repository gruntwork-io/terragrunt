//go:build windows
// +build windows

package exec

import (
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const InvalidHandleErrorMessage = "The handle is invalid"

// PrepareConsole enables support for escape sequences
// https://stackoverflow.com/questions/56460651/golang-fmt-print-033c-and-fmt-print-x1bc-are-not-clearing-screenansi-es
// https://github.com/containerd/console/blob/f652dc3/console_windows.go#L46
func PrepareConsole(logger log.Logger) {
	enableVirtualTerminalProcessing(logger, os.Stdin)
	enableVirtualTerminalProcessing(logger, os.Stderr)
	enableVirtualTerminalProcessing(logger, os.Stdout)
}

func enableVirtualTerminalProcessing(logger log.Logger, file *os.File) {
	var mode uint32
	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		if strings.Contains(err.Error(), InvalidHandleErrorMessage) {
			logger.Debugf("failed to get console mode: %v\n", err)
		} else {
			logger.Errorf("failed to get console mode: %v\n", err)
		}
		return
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		logger.Errorf("failed to set console mode: %v\n", err)
		if secondError := windows.SetConsoleMode(handle, mode); secondError != nil {
			logger.Errorf("failed to set console mode: %v\n", secondError)
			return
		}
	}
}

// For windows, there is no concept of a pseudoTTY so we run as if there is no pseudoTTY.
func runCommandWithPTY(logger log.Logger, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return errors.New(err)
	}
	return nil
}
