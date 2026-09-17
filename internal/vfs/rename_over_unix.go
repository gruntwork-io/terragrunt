//go:build !windows

package vfs

// RenameOver renames oldPath onto newPath, replacing whatever newPath names.
// Renaming needs write permission on the directory alone, so the mode of the
// entry being replaced does not matter here.
func RenameOver(fsys FS, oldPath, newPath string) error {
	return fsys.Rename(oldPath, newPath)
}
