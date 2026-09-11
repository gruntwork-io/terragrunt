//go:build windows

package vfs_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWindowsRenameOverReplacesReadOnlyFile pins that a read-only file, which
// MoveFileEx refuses to replace, is still replaced.
func TestWindowsRenameOverReplacesReadOnlyFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	source := filepath.Join(dir, "source.txt")

	require.NoError(t, vfs.WriteFile(fsys, target, []byte("old"), 0o644))
	require.NoError(t, fsys.Chmod(target, 0o444))
	require.NoError(t, vfs.WriteFile(fsys, source, []byte("new"), 0o644))

	require.NoError(t, vfs.RenameOver(fsys, source, target))

	contents, err := vfs.ReadFile(fsys, target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(contents))
	assert.False(t, vfs.Exists(fsys, source))
}

// TestWindowsWriteFileAtomicReplacesReadOnlyHardLink mirrors a lock file the
// CAS materialized as a read-only hard link into its store. The write must
// replace the link and leave the stored blob as it was.
func TestWindowsWriteFileAtomicReplacesReadOnlyHardLink(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := t.TempDir()
	blob := filepath.Join(dir, "blob")
	target := filepath.Join(dir, "target.txt")

	require.NoError(t, vfs.WriteFile(fsys, blob, []byte("stored"), 0o644))
	require.NoError(t, vfs.Link(fsys, blob, target))
	require.NoError(t, fsys.Chmod(blob, 0o444))

	require.NoError(t, vfs.WriteFileAtomic(fsys, target, []byte("new"), 0o644))

	contents, err := vfs.ReadFile(fsys, target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(contents))

	stored, err := vfs.ReadFile(fsys, blob)
	require.NoError(t, err)
	assert.Equal(t, "stored", string(stored))
}
