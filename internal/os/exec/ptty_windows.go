//go:build windows

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

// PrepareConsole enables support for escape sequences on Windows.
// Returns true if virtual terminal processing was successfully enabled on at least one output handle.
// https://stackoverflow.com/questions/56460651/golang-fmt-print-033c-and-fmt-print-x1bc-are-not-clearing-screenansi-es
// https://github.com/containerd/console/blob/f652dc3/console_windows.go#L46
func PrepareConsole(logger log.Logger) bool {
	enableVirtualTerminalInput(logger, os.Stdin)

	stdoutOK := enableVirtualTerminalProcessing(logger, os.Stdout)
	stderrOK := enableVirtualTerminalProcessing(logger, os.Stderr)

	if stdoutOK || stderrOK {
		return true
	}

	// If stdout/stderr are not console handles (e.g. pipes), try CONOUT$ directly.
	// CONOUT$ always refers to the active console output device. VT processing is a
	// screen buffer property that persists after the handle is closed, so enabling it
	// here affects all future console output even if stdout itself is a pipe.
	// Returning true is correct because stderr may still render to the console.
	conout, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		logger.Debugf("Could not open CONOUT$: %v", err)

		return false
	}
	defer conout.Close()

	return enableVirtualTerminalProcessing(logger, conout)
}

// enableVirtualTerminalInput sets ENABLE_VIRTUAL_TERMINAL_INPUT on an input handle (stdin).
// This is separate from enableVirtualTerminalProcessing because input and output handles
// use different flag values: ENABLE_VIRTUAL_TERMINAL_INPUT (0x200) for input vs
// ENABLE_VIRTUAL_TERMINAL_PROCESSING (0x4) for output.
// VT input is optional — failures are logged at Debug level (not Error) because
// missing VT input support does not break colored output or core functionality.
func enableVirtualTerminalInput(logger log.Logger, file *os.File) {
	var mode uint32

	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		logger.Debugf("failed to get console mode for input: %v", err)
		return
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
		logger.Debugf("virtual terminal input not supported: %v", err)
		// Restore original mode in case the failed call left the handle in a bad state.
		_ = windows.SetConsoleMode(handle, mode)
	}
}

// PrepareStdinForPrompt ensures stdin has the console mode flags required for
// interactive line input (line buffering, echo, processed input). Subprocesses
// on Windows can clear these flags, making stdin unusable for prompts.
func PrepareStdinForPrompt(logger log.Logger) {
	var mode uint32

	handle := windows.Handle(os.Stdin.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// stdin is not a console handle (e.g. pipe) — nothing to restore.
		return
	}

	required := uint32(windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	if mode&required != required {
		if err := windows.SetConsoleMode(handle, mode|required); err != nil {
			logger.Debugf("failed to restore stdin console mode for prompt: %v", err)
		}
	}
}

// enableVirtualTerminalProcessing sets ENABLE_VIRTUAL_TERMINAL_PROCESSING on an output handle
// (stdout or stderr) so that ANSI escape sequences are interpreted by the console.
// Returns true if the flag was successfully set.
func enableVirtualTerminalProcessing(logger log.Logger, file *os.File) bool {
	var mode uint32

	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		if strings.Contains(err.Error(), InvalidHandleErrorMessage) {
			logger.Debugf("failed to get console mode: %v", err)
		} else {
			logger.Errorf("failed to get console mode: %v", err)
		}

		return false
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		logger.Errorf("failed to set console mode: %v", err)
		_ = windows.SetConsoleMode(handle, mode)

		return false
	}

	return true
}

// ConsoleState stores the console mode for all standard handles so it can be restored
// after subprocess execution. Subprocesses on Windows can modify the console mode,
// which breaks ANSI escape handling and stdin line-input for the parent process.
//
// Note: SaveConsoleState and Restore operate on global OS handles (os.Stdin/Stdout/Stderr)
// without synchronization. This is practically safe for concurrent use in run --all
// because all goroutines target the same mode values. However, it is not formally
// synchronized via mutex — a goroutine that saves state after a subprocess has modified
// the console could capture a corrupted baseline. Impact is cosmetic only (garbled ANSI
// output, not data corruption).
type ConsoleState struct {
	stdinMode, stdoutMode, stderrMode uint32
	stdinOK, stdoutOK, stderrOK       bool
}

// SaveConsoleState captures the current console mode for stdin, stdout, and stderr.
func SaveConsoleState() ConsoleState {
	var s ConsoleState

	s.stdinOK = windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &s.stdinMode) == nil
	s.stdoutOK = windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &s.stdoutMode) == nil
	s.stderrOK = windows.GetConsoleMode(windows.Handle(os.Stderr.Fd()), &s.stderrMode) == nil

	return s
}

// Restore restores the saved console modes.
func (s ConsoleState) Restore() {
	if s.stdinOK {
		_ = windows.SetConsoleMode(windows.Handle(os.Stdin.Fd()), s.stdinMode)
	}

	if s.stdoutOK {
		_ = windows.SetConsoleMode(windows.Handle(os.Stdout.Fd()), s.stdoutMode)
	}

	if s.stderrOK {
		_ = windows.SetConsoleMode(windows.Handle(os.Stderr.Fd()), s.stderrMode)
	}
}

// For windows, there is no concept of a pseudoTTY so we run as if there is no pseudoTTY.
func runCommandWithPTY(logger log.Logger, cmd *exec.Cmd) error {
	logger.Debug("Running command without PTY")

	if err := cmd.Start(); err != nil {
		return errors.New(err)
	}
	return nil
}
