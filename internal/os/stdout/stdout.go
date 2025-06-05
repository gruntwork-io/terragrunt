// Package stdout provides utilities for working with stdout.
package stdout

import "os"

// IsRedirected returns true if the stdout is redirected.
func IsRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}
