//go:build !windows

package venv

// osStderrHandlesEscapes reports true: terminals outside Windows act on ANSI
// escape sequences.
func osStderrHandlesEscapes() bool {
	return true
}
