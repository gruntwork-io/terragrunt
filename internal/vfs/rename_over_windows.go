//go:build windows

package vfs

import (
	"errors"
	"io/fs"
	"os"
)

// writableFilePerms clears the read-only attribute, which is all a mode
// change can do to a file on Windows.
const writableFilePerms = os.FileMode(0o666)

// RenameOver renames oldPath onto newPath, replacing whatever newPath names.
// MoveFileEx refuses to replace a read-only file, so the attribute is cleared
// first. A hard link shares the attribute with every other name of the file,
// which stays writable under them.
func RenameOver(fsys FS, oldPath, newPath string) error {
	info, err := fsys.Stat(newPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o200 == 0 {
		if err := fsys.Chmod(newPath, writableFilePerms); err != nil {
			return err
		}
	}

	return fsys.Rename(oldPath, newPath)
}
