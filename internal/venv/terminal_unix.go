//go:build !windows

package venv

// osStderrHandlesEscapes returns true outside Windows.
func osStderrHandlesEscapes() bool {
	return true
}
