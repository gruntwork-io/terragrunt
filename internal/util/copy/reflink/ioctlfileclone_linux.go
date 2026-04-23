//go:build linux

package reflink

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func ioctlFileClone(from *os.File, toDir *os.File, toName string) (*os.File, error) {
	fromFD := int(from.Fd())

	fromInfo, err := from.Stat()
	if err != nil {
		return nil, err
	}

	toFile, err := createFile(toDir, toName, fromInfo.Mode())
	if err != nil {
		return toFile, err
	}
	toFD := int(toFile.Fd())

	err = unix.IoctlFileClone(toFD, fromFD)
	if err, ok := err.(syscall.Errno); ok {
		if _, ok := ioctlFileCloneNonRetryableErrors[err]; ok {
			return toFile, ErrCanNotReflink{wrapped: err}
		}
	}

	return toFile, err
}

var ioctlFileCloneNonRetryableErrors = map[syscall.Errno]bool{
	// see: https://man7.org/linux/man-pages/man2/FICLONE.2const.html
	syscall.EBADF:      true, // filesystem which src_fd resides on does not support reflink
	syscall.EINVAL:     true, // filesystem doesn't support reflinking the ranges of the given files, or either fd is a device / fifo / etc.
	syscall.EOPNOTSUPP: true, // filesystem doesn't support reflinking either file, or fd's refer to special inodes
	syscall.EXDEV:      true, // fd's are not on the same filesystem
}
