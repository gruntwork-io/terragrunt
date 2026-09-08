//go:build linux || darwin

package vfs

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// cloneUnsupportedErrnos are the errno values a copy-on-write clone reports
// when the filesystem cannot make one: a clone that would cross filesystems
// (EXDEV), a filesystem without reflink support (EOPNOTSUPP, ENOTSUP,
// ENOSYS), and a kernel that rejects the two files as a pair (EINVAL).
var cloneUnsupportedErrnos = []error{
	unix.EXDEV,
	unix.EOPNOTSUPP,
	unix.ENOTSUP,
	unix.EINVAL,
	unix.ENOSYS,
}

// cloneErr classifies a failed clone syscall. Callers distinguish a
// filesystem that cannot clone at all, which they answer by linking or
// copying instead, from a failure that would recur whatever they try, so
// only the first kind is reported as [ErrNoCloneFile].
func cloneErr(err error) error {
	for _, errno := range cloneUnsupportedErrnos {
		if errors.Is(err, errno) {
			return fmt.Errorf("%w: %w", ErrNoCloneFile, err)
		}
	}

	return err
}
