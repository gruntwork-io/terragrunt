//go:build windows

package venv

import (
	"os"

	"golang.org/x/sys/windows"
)

// osStderrHandlesEscapes reports whether the stderr console has virtual terminal processing enabled.
func osStderrHandlesEscapes() bool {
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stderr.Fd()), &mode); err != nil {
		return false
	}

	return mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
}
