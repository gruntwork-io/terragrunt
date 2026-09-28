//go:build !windows

package vfs

import "os"

// renameReplacing is a plain rename. POSIX rename replaces a name without
// consulting the mode of the file it names, and atomically, so concurrent
// renames onto one name each succeed.
func renameReplacing(oldname, newname string) error {
	return os.Rename(oldname, newname)
}
