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

// cloneErr wraps an errno in [cloneUnsupportedErrnos] with [ErrNoCloneFile]
// and returns any other error unchanged.
func cloneErr(err error) error {
	for _, errno := range cloneUnsupportedErrnos {
		if errors.Is(err, errno) {
			return fmt.Errorf("%w: %w", ErrNoCloneFile, err)
		}
	}

	return err
}
