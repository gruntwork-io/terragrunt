package reflink

import (
	"errors"
)

var ErrNotOnPlatform = errors.New("this function is not available on this platform")

// ErrCanNotReflink is returned when a reflink operation fails, not due to platform support.
// e.g. the source and destination are on different filesystems.
type ErrCanNotReflink struct {
	wrapped error
}

func (nr ErrCanNotReflink) Error() string {
	return "Reflink doesn't work here"
}

func (nr ErrCanNotReflink) Unwrap() error {
	return nr.wrapped
}

func (nr ErrCanNotReflink) Is(err error) bool {
	switch err.(type) {
	case ErrCanNotReflink:
		return true
	default:
		return false
	}
}
