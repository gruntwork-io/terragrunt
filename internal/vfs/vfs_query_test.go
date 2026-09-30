package vfs_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/data/file", []byte("x"), 0o644))

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{name: "regular file", path: "/data/file", want: true},
		{name: "directory", path: "/data", want: false},
		{name: "missing path", path: "/data/missing", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, vfs.IsFile(fsys, tc.path))
		})
	}
}

func TestEnsureDirectory(t *testing.T) {
	t.Parallel()

	t.Run("creates a missing directory and its parents private to the owner", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()

		require.NoError(t, vfs.EnsureDirectory(fsys, "/a/b/c"))

		info, err := fsys.Stat("/a/b/c")
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})

	t.Run("leaves an existing directory alone", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/dir/keep", []byte("x"), 0o644))

		require.NoError(t, vfs.EnsureDirectory(fsys, "/dir"))

		got, err := vfs.ReadFile(fsys, "/dir/keep")
		require.NoError(t, err)
		assert.Equal(t, []byte("x"), got)
	})

	t.Run("a file in the way is PathIsNotDirectory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/file", []byte("x"), 0o644))

		err := vfs.EnsureDirectory(fsys, "/data/file")

		var notDir vfs.PathIsNotDirectory
		require.ErrorAs(t, err, &notDir)
		assert.Equal(t, "/data/file is not a directory", err.Error())
	})

	t.Run("a filesystem refusing the directory reports why", func(t *testing.T) {
		t.Parallel()

		err := vfs.EnsureDirectory(afero.NewReadOnlyFs(vfs.NewMemMapFS()), "/data")

		require.ErrorIs(t, err, syscall.EPERM)
	})
}

func TestIsDirectoryEmpty(t *testing.T) {
	t.Parallel()

	t.Run("directory without entries is empty", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll("/empty", 0o755))

		empty, err := vfs.IsDirectoryEmpty(fsys, "/empty")
		require.NoError(t, err)
		assert.True(t, empty)
	})

	t.Run("directory holding a file is not empty", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/full/file", []byte("x"), 0o644))

		empty, err := vfs.IsDirectoryEmpty(fsys, "/full")
		require.NoError(t, err)
		assert.False(t, empty)
	})

	t.Run("missing directory is an error", func(t *testing.T) {
		t.Parallel()

		empty, err := vfs.IsDirectoryEmpty(vfs.NewMemMapFS(), "/missing")
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.False(t, empty)
	})

	t.Run("a failed close is reported instead of a verdict", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, base.MkdirAll("/empty", 0o755))

		fsys := &faultFS{FS: base, fileFaults: []string{faultClose}}

		empty, err := vfs.IsDirectoryEmpty(fsys, "/empty")
		require.ErrorIs(t, err, errInjected)
		assert.False(t, empty)
	})
}

func TestFileSHA256(t *testing.T) {
	t.Parallel()

	// Longer than one read block, so the hash spans several reads.
	large := bytes.Repeat([]byte("terragrunt"), 2000)

	testCases := []struct {
		name    string
		content []byte
	}{
		{name: "empty file", content: []byte{}},
		{name: "small file", content: []byte("hello")},
		{name: "file larger than one read block", content: large},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, "/file", tc.content, 0o644))

			got, err := vfs.FileSHA256(fsys, "/file")
			require.NoError(t, err)

			want := sha256.Sum256(tc.content)
			assert.Equal(t, want[:], got)
		})
	}

	t.Run("missing file is an error", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.FileSHA256(vfs.NewMemMapFS(), "/missing")
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	faultCases := []struct {
		name  string
		fault string
	}{
		{name: "failed read is an error", fault: faultRead},
		{name: "failed close is an error", fault: faultClose},
	}

	for _, tc := range faultCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(base, "/file", []byte("hello"), 0o644))

			fsys := &faultFS{FS: base, fileFaults: []string{tc.fault}}

			_, err := vfs.FileSHA256(fsys, "/file")
			require.ErrorIs(t, err, errInjected)
		})
	}
}

func TestReadFileSharingDelete(t *testing.T) {
	t.Parallel()

	t.Run("reads a file on the OS filesystem", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, []byte("on disk"), 0o644))

		got, err := vfs.ReadFileSharingDelete(vfs.NewOSFS(), path)
		require.NoError(t, err)
		assert.Equal(t, []byte("on disk"), got)
	})

	t.Run("missing file on the OS filesystem is not-exist", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.ReadFileSharingDelete(vfs.NewOSFS(), filepath.Join(t.TempDir(), "missing"))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("a filesystem without delete sharing reads plainly", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/file", []byte("in memory"), 0o644))

		got, err := vfs.ReadFileSharingDelete(fsys, "/file")
		require.NoError(t, err)
		assert.Equal(t, []byte("in memory"), got)
	})
}

func TestResolveForCompare(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/real/file", []byte("x"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/real", "/linked"))

	testCases := []struct {
		fsys vfs.FS
		name string
		path string
		want string
	}{
		{
			name: "existing path through a symlink resolves",
			fsys: fsys,
			path: "/linked/file",
			want: "/real/file",
		},
		{
			name: "missing path keeps its components under the resolved ancestor",
			fsys: fsys,
			path: "/linked/missing/deeper",
			want: "/real/missing/deeper",
		},
		{
			name: "path is cleaned before it is compared",
			fsys: fsys,
			path: "/real/./sub/../file",
			want: "/real/file",
		},
		{
			name: "nothing resolvable leaves the cleaned path",
			fsys: unresolvableFS{FS: vfs.NewMemMapFS()},
			path: "/a/../b/c",
			want: "/b/c",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, filepath.FromSlash(tc.want), vfs.ResolveForCompare(tc.fsys, tc.path))
		})
	}
}

func TestWithin(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/app/file", []byte("x"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, "/work/app-other/file", []byte("x"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, "/outside/file", []byte("x"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/outside", "/work/app/escape"))
	require.NoError(t, vfs.Symlink(fsys, "/work/app", "/shortcut"))

	testCases := []struct {
		name string
		dir  string
		path string
		want bool
	}{
		{name: "directory itself", dir: "/work/app", path: "/work/app", want: true},
		{name: "descendant", dir: "/work/app", path: "/work/app/file", want: true},
		{name: "missing descendant", dir: "/work/app", path: "/work/app/new/file", want: true},
		{name: "parent", dir: "/work/app", path: "/work", want: false},
		{name: "sibling sharing a name prefix", dir: "/work/app", path: "/work/app-other/file", want: false},
		{name: "symlink inside leading out", dir: "/work/app", path: "/work/app/escape/file", want: false},
		{name: "symlink outside leading in", dir: "/work/app", path: "/shortcut/file", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, vfs.Within(fsys, tc.dir, tc.path))
		})
	}

	t.Run("relative path cannot be placed against an absolute directory", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping on Windows: a rooted path without a drive is not absolute there")
		}

		assert.False(t, vfs.Within(fsys, "/work/app", "relative/file"))
	})
}

// errUnresolvable is what unresolvableFS reports for every path.
var errUnresolvable = errors.New("cannot resolve")

// unresolvableFS resolves no path at all, as a filesystem whose symlink
// evaluation fails everywhere would.
type unresolvableFS struct {
	vfs.FS
}

func (unresolvableFS) EvalSymlinksIfPossible(string) (string, bool, error) {
	return "", true, errUnresolvable
}
