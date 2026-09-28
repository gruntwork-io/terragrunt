//go:build windows

package vfs

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// readFileSharingDelete reads name through a handle that shares delete
// access. os.Open shares only read and write access, so Windows refuses it
// with a sharing violation while any other handle holds delete access.
func readFileSharingDelete(name string) (data []byte, err error) {
	path, err := verbatimPath(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	f := os.NewFile(uintptr(handle), name)

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	return io.ReadAll(f)
}
