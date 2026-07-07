// Package stdout provides utilities for working with stdout.
package stdout

import (
	"os"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// IsRedirected returns true if the stdout is redirected.
func IsRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}

// ShouldColor returns true if output written to stdout should be colored.
func ShouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !IsRedirected()
}
