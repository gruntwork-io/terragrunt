package run

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteNullVarsFile(t *testing.T) {
	t.Parallel()

	const (
		dir  = "/unit"
		path = dir + "/.terragrunt-null-vars.auto.tfvars.json"
	)

	t.Run("creates the file", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll(dir, 0755))

		require.NoError(t, writeNullVarsFile(fsys, path, []byte(`{"var1": null}`), true))

		contents, err := vfs.ReadFile(fsys, path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"var1": null}`, string(contents))
	})

	t.Run("in place keeps an existing file", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(`{"keep": "original"}`), 0644))

		err := writeNullVarsFile(fsys, path, []byte(`{"var1": null}`), true)

		var exists NullVarsFileExists

		require.ErrorAs(t, err, &exists)
		assert.Equal(t, path, exists.Path)

		contents, err := vfs.ReadFile(fsys, path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"keep": "original"}`, string(contents))
	})

	t.Run("in place keeps an existing symlink", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		target := filepath.Join(dir, "target.json")
		link := filepath.Join(dir, ".terragrunt-null-vars.auto.tfvars.json")

		require.NoError(t, os.WriteFile(target, []byte(`{"keep": "original"}`), 0644))
		require.NoError(t, os.Symlink("target.json", link))

		err := writeNullVarsFile(vfs.NewOSFS(), link, []byte(`{"var1": null}`), true)

		var exists NullVarsFileExists

		require.ErrorAs(t, err, &exists)

		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink)

		contents, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.JSONEq(t, `{"keep": "original"}`, string(contents))
	})

	t.Run("in the cache replaces an existing file", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(`{"var1":`), 0644))

		require.NoError(t, writeNullVarsFile(fsys, path, []byte(`{"var1": null}`), false))

		contents, err := vfs.ReadFile(fsys, path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"var1": null}`, string(contents))
	})

	t.Run("leaves nothing behind when the write fails", func(t *testing.T) {
		t.Parallel()

		fsys := failingWriteFS{FS: vfs.NewMemMapFS()}
		require.NoError(t, fsys.MkdirAll(dir, 0755))

		err := writeNullVarsFile(fsys, path, []byte(`{"var1": null}`), true)
		require.ErrorIs(t, err, errWriteFailed)

		// A partial file at path would make every later in-place run stop with NullVarsFileExists, and a
		// scratch file left next to it would clutter the unit directory.
		_, err = fsys.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist)

		d, err := fsys.Open(dir)
		require.NoError(t, err)

		names, err := d.Readdirnames(-1)
		require.NoError(t, err)
		assert.Empty(t, names)
	})
}

// errWriteFailed is what failingWriteFile reports, standing in for a full disk.
var errWriteFailed = errors.New("write failed")

// failingWriteFS opens files whose writes always fail.
type failingWriteFS struct {
	vfs.FS
}

func (fsys failingWriteFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	file, err := fsys.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return failingWriteFile{File: file}, nil
}

// failingWriteFile is an open file whose writes always fail.
type failingWriteFile struct {
	vfs.File
}

func (failingWriteFile) Write([]byte) (int, error) {
	return 0, errWriteFailed
}
