package vfs

import (
	"os"
	"syscall"

	"github.com/spf13/afero"
)

// cloneOp names the operation in the [os.LinkError] values the clone
// implementations return.
const cloneOp = "clonefile"

// FileCloner is an optional interface for filesystems that can produce a
// copy-on-write clone of a file.
type FileCloner interface {
	CloneFileIfPossible(oldname, newname string) error
}

// CloneFile creates newname as a copy-on-write clone of oldname: a file with
// its own inode and metadata that shares the original's data blocks until one
// of them is written.
//
// newname must not exist; the underlying system calls refuse to replace a
// file. A symlink at oldname is never followed. Filesystems and platforms
// without copy-on-write support report [ErrNoCloneFile].
func CloneFile(fsys FS, oldname, newname string) error {
	cloner, ok := fsys.(FileCloner)
	if !ok {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: ErrNoCloneFile}
	}

	return cloner.CloneFileIfPossible(oldname, newname)
}

// CloneFileIfPossible copies oldname to newname with the source's
// permissions. The in-memory filesystem stores file content as plain bytes
// with nothing to share between two files, so the clone is a full copy; it
// reports the same outcomes a copy-on-write clone would, and refuses a symlink
// at oldname with ELOOP as the Linux clone does.
//
// The content moves in one read and one write rather than through a copy
// buffer. Every write to an in-memory file reallocates and recopies the
// whole destination, so streaming a 4 MiB file in 32 KiB chunks costs seven
// times the time and nearly three times the allocation of reading it once.
func (fsys *memMapFS) CloneFileIfPossible(oldname, newname string) error {
	linkErr := func(err error) error {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: err}
	}

	oldResolved := fsys.resolveParent(oldname)
	newResolved := fsys.resolveParent(newname)

	if _, ok := fsys.readSymlink(oldResolved); ok {
		return linkErr(syscall.ELOOP)
	}

	if fsys.pathTaken(newResolved) {
		return linkErr(os.ErrExist)
	}

	info, err := fsys.Fs.Stat(oldResolved)
	if err != nil {
		return linkErr(err)
	}

	data, err := afero.ReadFile(fsys.Fs, oldResolved)
	if err != nil {
		return linkErr(err)
	}

	if err := afero.WriteFile(fsys.Fs, newResolved, data, info.Mode().Perm()); err != nil {
		return linkErr(err)
	}

	return nil
}
