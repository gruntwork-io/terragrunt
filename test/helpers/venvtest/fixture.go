package venvtest

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"

	"github.com/stretchr/testify/require"
)

const fixtureFileMode = 0o644

// NewFS returns an in-memory filesystem holding files, each path taken
// relative to root. Pair it with WithFS to give a command a tree to walk
// without staging one on disk.
func NewFS(t *testing.T, root string, files map[string]string) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()

	for path, contents := range files {
		require.NoError(
			t,
			vfs.WriteFile(fsys, filepath.Join(root, path), []byte(contents), fixtureFileMode),
		)
	}

	return fsys
}

// LoadFS mirrors the on-disk tree at dir into an in-memory filesystem and
// returns it with the root the copy landed at, for fixtures that are easier to
// keep as files than as literals. A subject that reached for os instead of the
// venv leaves the copy untouched and fails loudly.
//
// Only files are copied. Writing one registers its parent directories, so the
// tree arrives with them, but a directory holding nothing does not survive.
func LoadFS(t *testing.T, dir string) (vfs.FS, string) {
	t.Helper()

	const root = "/fixture"

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)

	src, dst := vfs.NewOSFS(), vfs.NewMemMapFS()

	require.NoError(t, vfs.WalkDir(src, abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		contents, err := vfs.ReadFile(src, path)
		if err != nil {
			return err
		}

		return vfs.WriteFile(dst, filepath.Join(root, rel), contents, info.Mode())
	}))

	return dst, root
}
