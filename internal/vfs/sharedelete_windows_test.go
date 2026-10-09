//go:build windows

package vfs_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// TestWindowsReadFileSharingDeleteWhileDeleteHandleOpen pins that a file
// stays readable while another handle holds delete access to it, as the
// handle of a rename that has just published the file does until it closes.
func TestWindowsReadFileSharingDeleteWhileDeleteHandleOpen(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "object")

	require.NoError(t, vfs.WriteFile(fsys, path, []byte("stored"), 0o644))

	pathPtr, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)

	handle, err := windows.CreateFile(
		pathPtr,
		windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, windows.CloseHandle(handle))
	})

	contents, err := vfs.ReadFileSharingDelete(fsys, path)
	require.NoError(t, err)
	assert.Equal(t, "stored", string(contents))
}

// TestWindowsReadFileSharingDeleteMissingIsNotExist pins that reading a
// missing file reports fs.ErrNotExist, which callers such as the CAS store
// rely on to tell a missing object from a failed read.
func TestWindowsReadFileSharingDeleteMissingIsNotExist(t *testing.T) {
	t.Parallel()

	_, err := vfs.ReadFileSharingDelete(vfs.NewOSFS(), filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, fs.ErrNotExist)
}
