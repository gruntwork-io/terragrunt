//go:build windows

package vfs

import (
	"errors"

	"golang.org/x/sys/windows"
)

// IsNameTooLong reports whether err is a filesystem error meaning the path was
// too long to name a file, as opposed to naming one that could not be reached.
//
// Windows reports an unusable name through its own error codes; the
// ENAMETOOLONG that other platforms return is defined but never produced here.
func IsNameTooLong(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_NAME) ||
		errors.Is(err, windows.ERROR_FILENAME_EXCED_RANGE)
}
