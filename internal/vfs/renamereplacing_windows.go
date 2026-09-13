//go:build windows

package vfs

import (
	"encoding/binary"
	"errors"
	"math/bits"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Offsets into FILE_RENAME_INFO. Its second field is a pointer-sized handle,
// so every field after the leading flags word shifts with the pointer size.
// The handle stays zero, and every Windows architecture is little-endian.
//
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_rename_info
const (
	renameInfoPtrSize       = bits.UintSize / 8
	renameInfoNameLenOffset = 2 * renameInfoPtrSize
	renameInfoNameOffset    = renameInfoNameLenOffset + 4
	utf16CharSize           = 2
)

// renameReplacing renames oldname onto newname through
// SetFileInformationByHandle. MoveFileEx, which os.Rename uses, refuses to
// replace a read-only file or a file another rename has just replaced.
func renameReplacing(oldname, newname string) error {
	if err := renameByHandle(oldname, newname); err != nil {
		return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: err}
	}

	return nil
}

func renameByHandle(oldname, newname string) error {
	oldPath, err := verbatimPath(oldname)
	if err != nil {
		return err
	}

	newPath, err := verbatimPath(newname)
	if err != nil {
		return err
	}

	buf, err := renameInfoBuffer(newPath)
	if err != nil {
		return err
	}

	oldPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}

	handle, err := windows.CreateFile(
		oldPtr,
		windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}

	setErr := windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, &buf[0], uint32(len(buf)))

	return errors.Join(setErr, windows.CloseHandle(handle))
}

// renameInfoBuffer returns a FILE_RENAME_INFO naming newPath.
func renameInfoBuffer(newPath string) ([]byte, error) {
	name, err := windows.UTF16FromString(newPath)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, renameInfoNameOffset+len(name)*utf16CharSize)

	// POSIX semantics unlink the replaced file at once, so a destination that
	// another rename has just replaced, or that a handle sharing delete access
	// holds open, can still be replaced. The read-only attribute is ignored
	// rather than cleared, since every hard link to the replaced file shares
	// it, e.g. a content store blob.
	binary.LittleEndian.PutUint32(buf, windows.FILE_RENAME_REPLACE_IF_EXISTS|
		windows.FILE_RENAME_POSIX_SEMANTICS|
		windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE)

	// The length counts bytes and leaves out the terminating NUL, which the
	// buffer still carries.
	binary.LittleEndian.PutUint32(buf[renameInfoNameLenOffset:], uint32((len(name)-1)*utf16CharSize))

	for i, char := range name {
		binary.LittleEndian.PutUint16(buf[renameInfoNameOffset+i*utf16CharSize:], char)
	}

	return buf, nil
}

// verbatimPath returns name as an absolute \\?\ path. The prefix lifts the
// MAX_PATH limit that applies otherwise unless long paths are enabled for the
// machine.
func verbatimPath(name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(abs, `\\?\`) {
		return abs, nil
	}

	if share, ok := strings.CutPrefix(abs, `\\`); ok {
		return `\\?\UNC\` + share, nil
	}

	return `\\?\` + abs, nil
}
