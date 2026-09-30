package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemMapFSCreate(t *testing.T) {
	t.Parallel()

	t.Run("creates through a symlinked directory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll("/real", 0o755))
		require.NoError(t, vfs.Symlink(fsys, "/real", "/linked"))

		file, err := fsys.Create("/linked/file")
		require.NoError(t, err)

		_, err = file.WriteString("created")
		require.NoError(t, err)
		require.NoError(t, file.Close())

		got, err := vfs.ReadFile(fsys, "/real/file")
		require.NoError(t, err)
		assert.Equal(t, []byte("created"), got)
	})
}

func TestMemMapFSNameCollisions(t *testing.T) {
	t.Parallel()

	t.Run("symlink onto an existing link is ErrExist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.Symlink(fsys, "/first", "/link"))

		err := vfs.Symlink(fsys, "/second", "/link")
		require.ErrorIs(t, err, os.ErrExist)

		target, err := vfs.Readlink(fsys, "/link")
		require.NoError(t, err)
		assert.Equal(t, "/first", target, "the existing link must keep its target")
	})

	t.Run("hard link onto an existing symlink is ErrExist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/file", []byte("x"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "/elsewhere", "/link"))

		require.ErrorIs(t, vfs.Link(fsys, "/file", "/link"), os.ErrExist)
	})

	t.Run("renaming a missing file is not-exist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()

		require.ErrorIs(t, fsys.Rename("/missing", "/renamed"), fs.ErrNotExist)
	})
}

func TestOSFSLockUnderMissingDirectory(t *testing.T) {
	t.Parallel()

	name := filepath.Join(t.TempDir(), "missing", "lock")

	t.Run("Lock", func(t *testing.T) {
		t.Parallel()

		unlocker, err := vfs.Lock(vfs.NewOSFS(), name)
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.Nil(t, unlocker)
	})

	t.Run("TryLock", func(t *testing.T) {
		t.Parallel()

		unlocker, acquired, err := vfs.TryLock(vfs.NewOSFS(), name)
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.False(t, acquired)
		assert.Nil(t, unlocker)
	})
}

func TestFSWorkersForGivesUpPastProbeBound(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: a missing path is answered by its drive")
	}

	// Deeper than the ancestors FSWorkersFor is willing to climb, and none of
	// them exist, so no probe answers before the climb gives up.
	missing := filepath.Join(t.TempDir(), strings.Repeat("missing"+string(filepath.Separator), 70))

	assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersFor(vfs.NewOSFS(), missing))
}
