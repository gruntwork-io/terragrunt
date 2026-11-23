//go:build (linux && !mips && !mipsle && !mips64 && !mips64le) || darwin

package vfs_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestStatusChangeOfMatchesAnUnchangedFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte("AAAA"), 0o644))

	first := statusChange(t, fsys, path)
	second := statusChange(t, fsys, path)

	assert.True(t, first.Equal(second))
}

func TestStatusChangeOfMovesWhenARewriteRestoresTheModTime(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte("AAAA"), 0o644))

	modTime := time.Now().Add(-time.Hour)
	require.NoError(t, fsys.Chtimes(path, modTime, modTime))

	before := statusChange(t, fsys, path)

	rewrite := filepath.Join(dir, "file.new")
	require.NoError(t, vfs.WriteFile(fsys, rewrite, []byte("BBBB"), 0o644))
	require.NoError(t, fsys.Chtimes(rewrite, modTime, modTime))
	require.NoError(t, vfs.RenameOver(fsys, rewrite, path))

	info, err := fsys.Stat(path)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(modTime), "the rewrite keeps the modification time")

	after := statusChange(t, fsys, path)

	assert.False(t, before.Equal(after))
}

// statusChange returns the status change of the file at path, which it requires the file's info to carry.
func statusChange(tb testing.TB, fsys vfs.FS, path string) vfs.StatusChange {
	tb.Helper()

	info, err := fsys.Stat(path)
	require.NoError(tb, err)

	change, ok := vfs.StatusChangeOf(info)
	require.True(tb, ok)

	return change
}
