//go:build !windows

package venv

// osStderrHandlesEscapes returns true, since terminals outside Windows support ANSI escape sequences.
func osStderrHandlesEscapes() bool {
	return true
}
