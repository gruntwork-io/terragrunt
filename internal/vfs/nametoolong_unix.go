//go:build !windows

package vfs

import (
	"errors"
	"syscall"
)

// IsNameTooLong reports whether err is a filesystem error meaning the path was
// too long to name a file, as opposed to naming one that could not be reached.
func IsNameTooLong(err error) bool {
	return errors.Is(err, syscall.ENAMETOOLONG)
}
