// +build windows

package shell

import (
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func PrepareConsole() {
	enableVirtualTerminalProcessing(os.Stderr)
	enableVirtualTerminalProcessing(os.Stdout)
}

func enableVirtualTerminalProcessing(file *os.File) {

	var mode uint32
	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		fmt.Printf("failed to get console mode: %v\n", err)
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		fmt.Printf("failed to set console mode: %v\n", err)
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
