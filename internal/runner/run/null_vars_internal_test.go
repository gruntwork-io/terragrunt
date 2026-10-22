package run

import (
	"errors"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteNullVarsFileExclusively(t *testing.T) {
	t.Parallel()

	const path = "/unit/.terragrunt-null-vars.auto.tfvars.json"

	t.Run("creates the file", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll("/unit", 0755))

		require.NoError(t, writeNullVarsFileExclusively(fsys, path, []byte(`{"var1": null}`)))

		contents, err := vfs.ReadFile(fsys, path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"var1": null}`, string(contents))
	})

	t.Run("keeps an existing file", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(`{"keep": "original"}`), 0644))

		err := writeNullVarsFileExclusively(fsys, path, []byte(`{"var1": null}`))

		var exists NullVarsFileExists

		require.ErrorAs(t, err, &exists)
		assert.Equal(t, path, exists.Path)

		contents, err := vfs.ReadFile(fsys, path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"keep": "original"}`, string(contents))
	})

	t.Run("removes the file when the write fails", func(t *testing.T) {
		t.Parallel()

		fsys := failingWriteFS{FS: vfs.NewMemMapFS()}
		require.NoError(t, fsys.MkdirAll("/unit", 0755))

		err := writeNullVarsFileExclusively(fsys, path, []byte(`{"var1": null}`))
		require.ErrorIs(t, err, errWriteFailed)

		// A partial file left behind would make every later run stop with NullVarsFileExists.
		_, err = fsys.Stat(path)
		assert.ErrorIs(t, err, os.ErrNotExist)
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
