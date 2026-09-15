//go:build !windows

package vfs

// renameOver is a plain rename. Renaming needs write permission on the
// directory alone, so the mode of the entry being replaced does not matter here.
func renameOver(fsys FS, oldPath, newPath string) error {
	return fsys.Rename(oldPath, newPath)
}
