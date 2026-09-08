package vfs

import (
	"os"

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
// file. Filesystems and platforms without copy-on-write support report that
// as [ErrNoCloneFile], which callers match with errors.Is to fall back to
// linking or copying. Any other error is a genuine failure.
func CloneFile(fsys FS, oldname, newname string) error {
	cloner, ok := fsys.(FileCloner)
	if !ok {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: ErrNoCloneFile}
	}

	return cloner.CloneFileIfPossible(oldname, newname)
}

// CloneFileIfPossible copies oldname to newname carrying the source's
// permissions. The in-memory filesystem stores file content as plain bytes
// with nothing to share between two files, so the clone is a full copy; it
// reports the same outcomes a copy-on-write clone would.
//
// The content moves in one read and one write rather than through a copy
// buffer. Every write to an in-memory file reallocates and recopies the
// whole destination, so streaming a 4 MiB file in 32 KiB chunks costs seven
// times the time and nearly three times the allocation of reading it once.
func (fsys *memMapFS) CloneFileIfPossible(oldname, newname string) error {
	linkErr := func(err error) error {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: err}
	}

	if _, err := fsys.Fs.Stat(newname); err == nil {
		return linkErr(os.ErrExist)
	}

	info, err := fsys.Fs.Stat(oldname)
	if err != nil {
		return linkErr(err)
	}

	data, err := afero.ReadFile(fsys.Fs, oldname)
	if err != nil {
		return linkErr(err)
	}

	if err := afero.WriteFile(fsys.Fs, newname, data, info.Mode().Perm()); err != nil {
		return linkErr(err)
	}

	return nil
}
