//go:build darwin

package reflink

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func clonefile(from *os.File, toDir *os.File, toName string) error {
	fromFD := int(from.Fd())
	toDirFD := int(toDir.Fd())
	err := unix.Fclonefileat(fromFD, toDirFD, toName, unix.CLONE_NOFOLLOW)

	if err, ok := err.(syscall.Errno); ok {
		if clonefileNonRetryableErrors[err] {
			return ErrCanNotReflink{wrapped: err}
		}
	}

	return err
}

var clonefileNonRetryableErrors = map[syscall.Errno]bool{
	syscall.EXDEV:   true, // "cross-device link"
	syscall.ENOTSUP: true, // "operation not supported"
}
