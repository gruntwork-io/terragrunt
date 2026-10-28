//go:build windows

package venv

import (
	"os"

	"golang.org/x/sys/windows"
)

// osStderrHandlesEscapes reports whether the console behind stderr has virtual
// terminal processing on, which Terragrunt asks for when it starts.
func osStderrHandlesEscapes() bool {
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stderr.Fd()), &mode); err != nil {
		return false
	}

	return mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
}
