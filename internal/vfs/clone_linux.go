//go:build linux

package vfs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// clonedFilePerms is the mode the destination is created with before the
// ioctl runs. The file has to be opened for writing to be cloned into, so it
// starts private to the caller and takes the source's permissions once the
// clone succeeds.
const clonedFilePerms = os.FileMode(0600)

// CloneFileIfPossible creates newname as a copy-on-write clone of oldname
// with the FICLONE ioctl, then gives it the source's permissions so the
// result matches a clone made on other platforms.
//
// btrfs and XFS formatted with reflink support implement the ioctl; ext4 and
// a clone that would cross filesystems reject it, and those rejections come
// back as [ErrNoCloneFile]. A symlink at oldname is refused with ELOOP.
func (fsys *osFS) CloneFileIfPossible(oldname, newname string) (err error) {
	src, err := os.OpenFile(oldname, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: err}
	}

	defer func() {
		err = errors.Join(err, src.Close())
	}()

	info, err := src.Stat()
	if err != nil {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: err}
	}

	dst, err := os.OpenFile(newname, os.O_WRONLY|os.O_CREATE|os.O_EXCL, clonedFilePerms)
	if err != nil {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: err}
	}

	if err := unix.IoctlFileClone(int(dst.Fd()), int(src.Fd())); err != nil {
		return &os.LinkError{
			Op:  cloneOp,
			Old: oldname,
			New: newname,
			Err: errors.Join(cloneErr(err), dst.Close(), os.Remove(newname)),
		}
	}

	if err := dst.Chmod(info.Mode().Perm()); err != nil {
		return &os.LinkError{
			Op:  cloneOp,
			Old: oldname,
			New: newname,
			Err: errors.Join(err, dst.Close(), os.Remove(newname)),
		}
	}

	if err := dst.Close(); err != nil {
		return &os.LinkError{
			Op:  cloneOp,
			Old: oldname,
			New: newname,
			Err: errors.Join(err, os.Remove(newname)),
		}
	}

	return nil
}
