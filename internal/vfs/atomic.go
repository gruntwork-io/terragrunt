package vfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// atomicTempPattern names the scratch file after its destination so one left
// behind is traceable, with the random component [CreateTemp] substitutes for "*".
const atomicTempPattern = ".*.tmp"

// WriteFileAtomic writes data to path through a scratch file that replaces path
// only once the content is on disk. See [StreamFileAtomic] for what that buys and
// how perm is applied.
func WriteFileAtomic(fsys FS, path string, data []byte, perm os.FileMode) error {
	return StreamFileAtomic(fsys, path, perm, func(w io.Writer) error {
		_, err := w.Write(data)

		return err
	})
}

// StreamFileAtomic passes a writer to write and renames what it produces onto
// path, creating path's parent directories if they are missing. The scratch file
// shares path's directory so the rename stays on one filesystem.
//
// Three properties come out of this that a plain [WriteFile] does not have. A
// reader never sees a half-written file, and a failed write leaves the previous
// one untouched rather than truncated. A symlink at path is replaced rather than
// followed, so the content lands where the caller named. An existing file takes
// perm rather than keeping the mode it already had, which a create cannot do.
//
// perm is applied exactly, not masked by the process umask, so a caller asking
// for a mode wider than 0600 gets it whatever the umask would have allowed.
func StreamFileAtomic(fsys FS, path string, perm os.FileMode, write func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := fsys.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	file, err := CreateTemp(fsys, dir, filepath.Base(path)+atomicTempPattern)
	if err != nil {
		return err
	}

	tmpPath := file.Name()

	if err := fsys.Chmod(tmpPath, perm); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := write(file); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := file.Close(); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	if err := fsys.Rename(tmpPath, path); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	return nil
}
