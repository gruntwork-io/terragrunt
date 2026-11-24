package vfs

import (
	"io/fs"
	"time"
)

// StatusChange identifies one version of a file by the device and inode the file lives at and the time its contents
// or metadata last changed. No system call sets the change time, so a write moves it even when the writer restores
// the file's modification time, as `touch -r` does, and a file renamed over another keeps its own inode.
type StatusChange struct {
	// Time is when the file's contents or metadata last changed.
	Time time.Time
	// Device is the ID of the device the file lives on.
	Device uint64
	// Inode is the number of the file on Device.
	Inode uint64
}

// Equal reports whether c and o identify the same version of a file.
func (c StatusChange) Equal(o StatusChange) bool {
	return c.Device == o.Device && c.Inode == o.Inode && c.Time.Equal(o.Time)
}

// StatusChangeOf returns the [StatusChange] of the file info describes, and false when info does not carry one, as
// on Windows and for a file in a [NewMemMapFS].
func StatusChangeOf(info fs.FileInfo) (StatusChange, bool) {
	return statusChangeOf(info)
}
