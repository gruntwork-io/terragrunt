//go:build linux

package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneFileOSFS(t *testing.T) {
	t.Parallel()

	t.Run("clone matches the source or is refused as unsupported", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		clone := filepath.Join(dir, "clone")

		require.NoError(t, os.WriteFile(source, []byte("source"), 0o640))
		require.NoError(t, os.Chmod(source, 0o640))

		err := vfs.CloneFile(vfs.NewOSFS(), source, clone)

		// ext4 and tmpfs have no copy-on-write clone; btrfs and XFS do.
		if err != nil {
			require.ErrorIs(t, err, vfs.ErrNoCloneFile)

			_, statErr := os.Lstat(clone)
			require.ErrorIs(t, statErr, fs.ErrNotExist, "a refused clone must not leave the destination behind")

			return
		}

		got, err := os.ReadFile(clone)
		require.NoError(t, err)
		assert.Equal(t, []byte("source"), got)

		info, err := os.Stat(clone)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	})

	t.Run("missing source is not-exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := vfs.CloneFile(vfs.NewOSFS(), filepath.Join(dir, "missing"), filepath.Join(dir, "clone"))

		var linkErr *os.LinkError
		require.ErrorAs(t, err, &linkErr)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("symlink source is ELOOP", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		link := filepath.Join(dir, "link")

		require.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
		require.NoError(t, os.Symlink(source, link))

		err := vfs.CloneFile(vfs.NewOSFS(), link, filepath.Join(dir, "clone"))
		require.ErrorIs(t, err, syscall.ELOOP)
	})

	t.Run("existing destination is ErrExist and is left alone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		clone := filepath.Join(dir, "clone")

		require.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
		require.NoError(t, os.WriteFile(clone, []byte("taken"), 0o644))

		err := vfs.CloneFile(vfs.NewOSFS(), source, clone)
		require.ErrorIs(t, err, os.ErrExist)

		got, err := os.ReadFile(clone)
		require.NoError(t, err)
		assert.Equal(t, []byte("taken"), got)
	})
}
