package vfs_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	t.Run("copies contents and permissions", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			contents []byte
			mode     os.FileMode
		}{
			{
				name:     "text contents",
				contents: []byte("hello world"),
				mode:     0o644,
			},
			{
				name:     "empty file",
				contents: []byte{},
				mode:     0o600,
			},
			{
				name:     "binary contents",
				contents: []byte{0x00, 0x01, 0xfe, 0xff},
				mode:     0o755,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fsys := vfs.NewMemMapFS()
				require.NoError(t, vfs.WriteFile(fsys, "/data/source", tc.contents, tc.mode))

				require.NoError(t, vfs.CopyFile(fsys, "/data/source", "/data/destination"))

				copied, err := vfs.ReadFile(fsys, "/data/destination")
				require.NoError(t, err)
				assert.Equal(t, tc.contents, copied)

				info, err := fsys.Stat("/data/destination")
				require.NoError(t, err)
				assert.Equal(t, tc.mode, info.Mode().Perm())
			})
		}
	})

	t.Run("closes the source file", func(t *testing.T) {
		t.Parallel()

		fsys := &trackingFS{FS: vfs.NewMemMapFS()}
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("payload"), 0o644))

		require.NoError(t, vfs.CopyFile(fsys, "/data/source", "/data/destination"))

		opened := fsys.openedFiles()
		require.Len(t, opened, 1)
		assert.True(t, opened[0].closed.Load())
	})

	t.Run("missing source returns fs.ErrNotExist", func(t *testing.T) {
		t.Parallel()

		err := vfs.CopyFile(vfs.NewMemMapFS(), "/data/missing", "/data/destination")

		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestWriteFileWithSamePermissions(t *testing.T) {
	t.Parallel()

	t.Run("replaces the destination with the source mode", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			mode os.FileMode
		}{
			{
				name: "owner only",
				mode: 0o600,
			},
			{
				name: "world readable",
				mode: 0o644,
			},
			{
				name: "executable",
				mode: 0o755,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fsys := vfs.NewMemMapFS()
				require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("source"), tc.mode))
				require.NoError(t, vfs.WriteFile(fsys, "/data/destination", []byte("stale"), 0o400))

				err := vfs.WriteFileWithSamePermissions(
					fsys,
					"/data/source",
					"/data/destination",
					bytes.NewReader([]byte("fresh")),
				)
				require.NoError(t, err)

				info, err := fsys.Stat("/data/destination")
				require.NoError(t, err)
				assert.Equal(t, tc.mode, info.Mode().Perm())

				contents, err := vfs.ReadFile(fsys, "/data/destination")
				require.NoError(t, err)
				assert.Equal(t, []byte("fresh"), contents)
			})
		}
	})

	t.Run("overwrites a read-only destination", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")

		require.NoError(t, vfs.WriteFile(fsys, source, []byte("fresh"), 0o644))
		require.NoError(t, vfs.WriteFile(fsys, destination, []byte("stale"), 0o400))

		err := vfs.WriteFileWithSamePermissions(
			fsys,
			source,
			destination,
			bytes.NewReader([]byte("fresh")),
		)
		require.NoError(t, err)

		contents, err := vfs.ReadFile(fsys, destination)
		require.NoError(t, err)
		assert.Equal(t, []byte("fresh"), contents)

		info, err := fsys.Stat(destination)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o200)
	})

	t.Run("missing source returns fs.ErrNotExist", func(t *testing.T) {
		t.Parallel()

		err := vfs.WriteFileWithSamePermissions(
			vfs.NewMemMapFS(),
			"/data/missing",
			"/data/destination",
			bytes.NewReader(nil),
		)

		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("filesystem refusing removal returns fs.ErrPermission", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(base, "/data/source", []byte("source"), 0o644))

		err := vfs.WriteFileWithSamePermissions(
			afero.NewReadOnlyFs(base),
			"/data/source",
			"/data/destination",
			bytes.NewReader([]byte("fresh")),
		)

		require.ErrorIs(t, err, fs.ErrPermission)
	})

	t.Run("missing destination directory returns fs.ErrNotExist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		dir := t.TempDir()
		source := filepath.Join(dir, "source")

		require.NoError(t, vfs.WriteFile(fsys, source, []byte("fresh"), 0o644))

		err := vfs.WriteFileWithSamePermissions(
			fsys,
			source,
			filepath.Join(dir, "missing", "destination"),
			bytes.NewReader([]byte("fresh")),
		)

		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestIsOSFS(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fsys     vfs.FS
		name     string
		expected bool
	}{
		{
			fsys:     vfs.NewOSFS(),
			name:     "OS filesystem",
			expected: true,
		},
		{
			fsys:     vfs.NewMemMapFS(),
			name:     "in-memory filesystem",
			expected: false,
		},
		{
			fsys:     afero.NewReadOnlyFs(vfs.NewOSFS()),
			name:     "wrapped OS filesystem",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, vfs.IsOSFS(tc.fsys))
		})
	}
}

func TestIsDir(t *testing.T) {
	t.Parallel()

	t.Run("in-memory paths", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			path     string
			expected bool
		}{
			{
				name:     "directory",
				path:     "/root/dir",
				expected: true,
			},
			{
				name:     "file",
				path:     "/root/file.txt",
				expected: false,
			},
			{
				name:     "missing path",
				path:     "/root/missing",
				expected: false,
			},
			{
				name:     "missing parent",
				path:     "/root/missing/child",
				expected: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tc.expected, vfs.IsDir(newPopulatedFS(t), tc.path))
			})
		}
	})

	t.Run("follows a symlink to a directory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		dir := t.TempDir()

		require.NoError(t, fsys.MkdirAll(filepath.Join(dir, "real"), 0o755))
		require.NoError(t, vfs.Symlink(fsys, filepath.Join(dir, "real"), filepath.Join(dir, "link")))

		assert.True(t, vfs.IsDir(fsys, filepath.Join(dir, "link")))
	})
}

func TestLink(t *testing.T) {
	t.Parallel()

	t.Run("links contents and mode on MemMapFS", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("linked"), 0o600))

		require.NoError(t, vfs.Link(fsys, "/data/source", "/data/link"))

		contents, err := vfs.ReadFile(fsys, "/data/link")
		require.NoError(t, err)
		assert.Equal(t, []byte("linked"), contents)

		info, err := fsys.Stat("/data/link")
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("shares an inode on OSFS", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		link := filepath.Join(dir, "link")

		require.NoError(t, vfs.WriteFile(fsys, source, []byte("linked"), 0o644))
		require.NoError(t, vfs.Link(fsys, source, link))

		sourceInfo, err := fsys.Stat(source)
		require.NoError(t, err)

		linkInfo, err := fsys.Stat(link)
		require.NoError(t, err)

		assert.True(t, os.SameFile(sourceInfo, linkInfo))
	})

	t.Run("existing destination returns os.ErrExist", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/data/source", []byte("source"), 0o644))
		require.NoError(t, vfs.WriteFile(fsys, "/data/link", []byte("taken"), 0o644))

		require.ErrorIs(t, vfs.Link(fsys, "/data/source", "/data/link"), os.ErrExist)
	})

	t.Run("missing source returns fs.ErrNotExist", func(t *testing.T) {
		t.Parallel()

		err := vfs.Link(vfs.NewMemMapFS(), "/data/missing", "/data/link")

		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("filesystem without hard links returns ErrNoHardLink", func(t *testing.T) {
		t.Parallel()

		err := vfs.Link(afero.NewReadOnlyFs(vfs.NewMemMapFS()), "/data/source", "/data/link")

		require.ErrorIs(t, err, vfs.ErrNoHardLink)
	})
}

func TestReadlink(t *testing.T) {
	t.Parallel()

	t.Run("reads a target on OSFS", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		dir := t.TempDir()
		target := filepath.Join(dir, "target.txt")
		link := filepath.Join(dir, "link.txt")

		require.NoError(t, vfs.WriteFile(fsys, target, []byte("target"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, target, link))

		resolved, err := vfs.Readlink(fsys, link)

		require.NoError(t, err)
		assert.Equal(t, target, resolved)
	})

	t.Run("regular file on OSFS returns an error", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		target := filepath.Join(t.TempDir(), "target.txt")

		require.NoError(t, vfs.WriteFile(fsys, target, []byte("target"), 0o644))

		_, err := vfs.Readlink(fsys, target)

		require.Error(t, err)
	})

	t.Run("reads a target on MemMapFS", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.Symlink(fsys, "/root/target.txt", "/root/link.txt"))

		resolved, err := vfs.Readlink(fsys, "/root/link.txt")

		require.NoError(t, err)
		assert.Equal(t, "/root/target.txt", resolved)
	})

	t.Run("filesystem without symlinks returns ErrNoSymlink", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.Readlink(afero.NewMemMapFs(), "/root/link.txt")

		require.ErrorIs(t, err, afero.ErrNoSymlink)
	})
}

func TestTryLockOSFS(t *testing.T) {
	t.Parallel()

	t.Run("acquires a free lock", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		lockPath := filepath.Join(t.TempDir(), "test.lock")

		lock, acquired, err := vfs.TryLock(fsys, lockPath)

		require.NoError(t, err)
		require.True(t, acquired)
		require.NotNil(t, lock)
		require.NoError(t, lock.Unlock())
	})

	t.Run("reports a lock held elsewhere", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewOSFS()
		lockPath := filepath.Join(t.TempDir(), "test.lock")

		held, err := vfs.Lock(fsys, lockPath)
		require.NoError(t, err)
		t.Cleanup(func() { _ = held.Unlock() })

		lock, acquired, err := vfs.TryLock(fsys, lockPath)

		require.NoError(t, err)
		assert.False(t, acquired)
		assert.Nil(t, lock)
	})
}

func TestWithFollowSymlinks(t *testing.T) {
	t.Parallel()

	t.Run("descends into a symlinked directory", func(t *testing.T) {
		t.Parallel()

		root := newSymlinkedTree(t)

		seen := walkParallelPaths(t, root, vfs.WithFollowSymlinks())

		assert.Contains(t, seen, filepath.Join(root, "link", "file.txt"))
		assert.Contains(t, seen, filepath.Join(root, "real", "file.txt"))
	})

	t.Run("without the option a symlink stays a leaf", func(t *testing.T) {
		t.Parallel()

		root := newSymlinkedTree(t)

		seen := walkParallelPaths(t, root)

		assert.Contains(t, seen, filepath.Join(root, "link"))
		assert.NotContains(t, seen, filepath.Join(root, "link", "file.txt"))
	})
}

func TestLstatSymlink(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/root/target.txt", []byte("target"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/root/target.txt", "/root/link.txt"))

	info, err := vfs.Lstat(fsys, "/root/link.txt")
	require.NoError(t, err)

	assert.Equal(t, "link.txt", info.Name())
	assert.Zero(t, info.Size())
	assert.True(t, info.ModTime().IsZero())
	assert.False(t, info.IsDir())
	assert.Nil(t, info.Sys())
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestFileInfoDirEntry(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/root/file.txt", []byte("data"), 0o644))

	stat, err := fsys.Stat("/root/file.txt")
	require.NoError(t, err)

	entry := vfs.FileInfoDirEntry{FileInfo: stat}

	info, err := entry.Info()
	require.NoError(t, err)

	assert.Equal(t, stat, info)
	assert.Equal(t, "file.txt", entry.Name())
	assert.False(t, entry.IsDir())
	assert.Zero(t, entry.Type())
}

func TestMemMapFSRemove(t *testing.T) {
	t.Parallel()

	t.Run("Remove deletes a symlink and keeps its target", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/root/target.txt", []byte("target"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "/root/target.txt", "/root/link.txt"))

		require.NoError(t, fsys.Remove("/root/link.txt"))

		_, err := vfs.Readlink(fsys, "/root/link.txt")
		require.Error(t, err)

		exists, err := vfs.FileExists(fsys, "/root/target.txt")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("RemoveAll deletes a symlink and keeps its target", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/root/target.txt", []byte("target"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "/root/target.txt", "/root/link.txt"))

		require.NoError(t, fsys.RemoveAll("/root/link.txt"))

		_, err := vfs.Readlink(fsys, "/root/link.txt")
		require.Error(t, err)

		exists, err := vfs.FileExists(fsys, "/root/target.txt")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("RemoveAll deletes a directory tree", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/root/tree/nested/file.txt", []byte("f"), 0o644))

		require.NoError(t, fsys.RemoveAll("/root/tree"))

		exists, err := vfs.FileExists(fsys, "/root/tree/nested/file.txt")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestNoSymlinkFS(t *testing.T) {
	t.Parallel()

	fsys := &vfs.NoSymlinkFS{FS: vfs.NewMemMapFS()}

	err := vfs.Symlink(fsys, "/root/target.txt", "/root/link.txt")

	require.ErrorIs(t, err, afero.ErrNoSymlink)

	var linkErr *os.LinkError

	require.ErrorAs(t, err, &linkErr)
	assert.Equal(t, "symlink", linkErr.Op)
}

func TestEvalSymlinksRelativePaths(t *testing.T) {
	t.Parallel()

	t.Run("resolves a parent reference", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll("/root/first", 0o755))
		require.NoError(t, fsys.MkdirAll("/root/second", 0o755))

		resolved, err := vfs.EvalSymlinks(fsys, "/root/first/../second")

		require.NoError(t, err)
		assert.Equal(t, "/root/second", resolved)
	})

	t.Run("resolves a parent reference above the root", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll("/root", 0o755))

		resolved, err := vfs.EvalSymlinks(fsys, "/root/..")

		require.NoError(t, err)
		assert.Equal(t, string(filepath.Separator), resolved)
	})

	t.Run("resolves a relative symlink target", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/root/target.txt", []byte("target"), 0o644))
		require.NoError(t, vfs.Symlink(fsys, "target.txt", "/root/link.txt"))

		resolved, err := vfs.EvalSymlinks(fsys, "/root/link.txt")

		require.NoError(t, err)
		assert.Equal(t, "/root/target.txt", resolved)
	})
}

// newPopulatedFS returns an in-memory filesystem holding one directory and one file.
func newPopulatedFS(t *testing.T) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll("/root/dir", 0o755))
	require.NoError(t, vfs.WriteFile(fsys, "/root/file.txt", []byte("file"), 0o644))

	return fsys
}

// newSymlinkedTree returns an OS temp dir holding a real directory with one
// file in it, and a symlink pointing at that directory.
func newSymlinkedTree(t *testing.T) string {
	t.Helper()

	fsys := vfs.NewOSFS()
	root := t.TempDir()

	require.NoError(t, fsys.MkdirAll(filepath.Join(root, "real"), 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(root, "real", "file.txt"), []byte("f"), 0o644),
	)
	require.NoError(
		t,
		vfs.Symlink(fsys, filepath.Join(root, "real"), filepath.Join(root, "link")),
	)

	return root
}

// walkParallelPaths walks root on the OS filesystem and returns every path the
// walk reported.
func walkParallelPaths(
	t *testing.T,
	root string,
	opts ...vfs.WalkDirParallelOption,
) map[string]struct{} {
	t.Helper()

	var mu sync.Mutex

	seen := map[string]struct{}{}

	err := vfs.WalkDirParallel(
		vfs.NewOSFS(),
		root,
		func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			seen[path] = struct{}{}

			return nil
		},
		opts...,
	)
	require.NoError(t, err)

	return seen
}

// trackingFS records every file Open hands out so a test can assert the caller
// closed it.
type trackingFS struct {
	vfs.FS
	opened []*trackedFile
	mu     sync.Mutex
}

func (fsys *trackingFS) Open(name string) (vfs.File, error) {
	file, err := fsys.FS.Open(name)
	if err != nil {
		return nil, err
	}

	tracked := &trackedFile{File: file}

	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	fsys.opened = append(fsys.opened, tracked)

	return tracked, nil
}

func (fsys *trackingFS) openedFiles() []*trackedFile {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	return slices.Clone(fsys.opened)
}

// trackedFile flags itself closed so trackingFS can report unclosed handles.
type trackedFile struct {
	vfs.File
	closed atomic.Bool
}

func (file *trackedFile) Close() error {
	file.closed.Store(true)

	return file.File.Close()
}
