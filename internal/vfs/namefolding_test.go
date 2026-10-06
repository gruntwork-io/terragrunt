package vfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestDetectNameFoldingMemMapFS(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll("/dir", 0o755))

	folding, err := vfs.DetectNameFolding(fsys, "/dir")
	require.NoError(t, err)
	assert.Equal(t, vfs.NameFolding{}, folding, "the in-memory filesystem keeps every spelling distinct")

	entries, err := vfs.ReadDir(fsys, "/dir")
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file must be removed")
}

// The real filesystem is the only place the answer varies, so the expectation comes from an independent lookup.
func TestDetectNameFoldingOSFS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "reference-\u00e9.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))

	want := vfs.NameFolding{
		Case:    existsOnDisk(t, filepath.Join(dir, "REFERENCE-\u00e9.TXT")),
		Unicode: existsOnDisk(t, filepath.Join(dir, norm.NFD.String(name))),
	}

	folding, err := vfs.DetectNameFolding(vfs.NewOSFS(), dir)
	require.NoError(t, err)
	assert.Equal(t, want, folding)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the reference file may remain")
}

func existsOnDisk(t *testing.T, path string) bool {
	t.Helper()

	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}

	require.NoError(t, err)

	return true
}
