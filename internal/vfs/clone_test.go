package vfs_test

import (
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneFile(t *testing.T) {
	t.Parallel()

	t.Run("clone has the source's content and permissions", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("source"), 0o444))

		require.NoError(t, vfs.CloneFile(fsys, "/data/source", "/data/clone"))

		got, err := vfs.ReadFile(fsys, "/data/clone")
		require.NoError(t, err)
		assert.Equal(t, []byte("source"), got)

		info, err := fsys.Stat("/data/clone")
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())
	})

	t.Run("existing destination returns os.ErrExist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("source"), 0o644))
		require.NoError(t, vfs.WriteFile(fsys, "/data/clone", []byte("taken"), 0o644))

		require.ErrorIs(t, vfs.CloneFile(fsys, "/data/source", "/data/clone"), os.ErrExist)
	})

	t.Run("missing source returns fs.ErrNotExist", func(t *testing.T) {
		t.Parallel()

		err := vfs.CloneFile(vfs.NewMemMapFS(), "/data/missing", "/data/clone")

		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("symlink source returns ELOOP", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("source"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "/data/source", "/data/link"))

		require.ErrorIs(t, vfs.CloneFile(fsys, "/data/link", "/data/clone"), syscall.ELOOP)
	})

	t.Run("clones through a symlinked directory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/real/source", []byte("source"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "/real", "/linked"))

		require.NoError(t, vfs.CloneFile(fsys, "/linked/source", "/linked/clone"))

		got, err := vfs.ReadFile(fsys, "/real/clone")
		require.NoError(t, err)
		assert.Equal(t, []byte("source"), got)
	})

	t.Run("filesystem without clone support returns ErrNoCloneFile", func(t *testing.T) {
		t.Parallel()

		err := vfs.CloneFile(afero.NewReadOnlyFs(vfs.NewMemMapFS()), "/data/source", "/data/clone")

		require.ErrorIs(t, err, vfs.ErrNoCloneFile)
	})
}
