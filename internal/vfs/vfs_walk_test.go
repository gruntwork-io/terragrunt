package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkDirStopsEarly(t *testing.T) {
	t.Parallel()

	fsys := newWalkTree(t)

	testCases := []struct {
		stopAt string
		stopBy error
		name   string
		want   []string
	}{
		{
			name:   "SkipAll ends the walk without an error",
			stopAt: "/root/a.txt",
			stopBy: filepath.SkipAll,
			want:   []string{"/root", "/root/a.txt"},
		},
		{
			name:   "SkipDir from a file skips the rest of its directory",
			stopAt: "/root/sub/inner.txt",
			stopBy: filepath.SkipDir,
			want:   []string{"/root", "/root/a.txt", "/root/sub", "/root/sub/inner.txt", "/root/z.txt"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var visited []string

			err := vfs.WalkDir(fsys, "/root", func(path string, _ fs.DirEntry, err error) error {
				require.NoError(t, err)

				visited = append(visited, filepath.ToSlash(path))

				if filepath.ToSlash(path) == tc.stopAt {
					return tc.stopBy
				}

				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, visited)
		})
	}
}

func TestWalkDirUnreadableDirectory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		onError error
		wantErr error
		name    string
		want    []string
	}{
		{
			name:    "callback ignoring the error keeps walking",
			onError: nil,
			want:    []string{"/root", "/root/a.txt", "/root/sub", "/root/sub", "/root/z.txt"},
		},
		{
			name:    "callback skipping the directory keeps walking",
			onError: filepath.SkipDir,
			want:    []string{"/root", "/root/a.txt", "/root/sub", "/root/sub", "/root/z.txt"},
		},
		{
			name:    "callback returning the error ends the walk with it",
			onError: errInjected,
			wantErr: errInjected,
			want:    []string{"/root", "/root/a.txt", "/root/sub", "/root/sub"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := &faultFS{FS: newWalkTree(t), faults: map[string]string{faultOpen: "/root/sub"}}

			var (
				visited []string
				errs    []error
			)

			err := vfs.WalkDir(fsys, "/root", func(path string, _ fs.DirEntry, err error) error {
				visited = append(visited, filepath.ToSlash(path))

				if err != nil {
					errs = append(errs, err)
					return tc.onError
				}

				return nil
			})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.want, visited)
			require.Len(t, errs, 1)
			require.ErrorIs(t, errs[0], errInjected)
		})
	}
}

func TestWalkDirParallelSkipAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
	}

	var files atomic.Int32

	err := vfs.WalkDirParallel(vfs.NewOSFS(), root, func(_ string, d fs.DirEntry, err error) error {
		// fastwalk hands the stop back to callbacks still in flight.
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		files.Add(1)

		return filepath.SkipAll
	}, vfs.WithWorkers(1))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, files.Load(), int32(1))
}

func TestWalkDirWithSymlinksEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("SkipAll ends the walk without an error", func(t *testing.T) {
		t.Parallel()

		var visited []string

		err := vfs.WalkDirWithSymlinks(newWalkTree(t), "/root", func(path string, _ fs.DirEntry, err error) error {
			require.NoError(t, err)

			visited = append(visited, filepath.ToSlash(path))

			if filepath.ToSlash(path) == "/root/a.txt" {
				return filepath.SkipAll
			}

			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"/root", "/root/a.txt"}, visited)
	})

	t.Run("root the callback skips ends the walk without an error", func(t *testing.T) {
		t.Parallel()

		var failed []string

		err := vfs.WalkDirWithSymlinks(vfs.NewMemMapFS(), "/missing", func(path string, _ fs.DirEntry, err error) error {
			require.ErrorIs(t, err, fs.ErrNotExist)

			failed = append(failed, filepath.ToSlash(path))

			return filepath.SkipDir
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"/missing"}, failed)
	})

	t.Run("callback error ends the walk with it", func(t *testing.T) {
		t.Parallel()

		err := vfs.WalkDirWithSymlinks(newWalkTree(t), "/root", func(path string, _ fs.DirEntry, _ error) error {
			if filepath.ToSlash(path) == "/root/a.txt" {
				return errInjected
			}

			return nil
		})
		require.ErrorIs(t, err, errInjected)
	})

	t.Run("unreadable directory is handed to the callback", func(t *testing.T) {
		t.Parallel()

		fsys := &faultFS{FS: newWalkTree(t), faults: map[string]string{faultOpen: "/root/sub"}}

		var failed []string

		err := vfs.WalkDirWithSymlinks(fsys, "/root", func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				require.ErrorIs(t, err, errInjected)

				failed = append(failed, filepath.ToSlash(path))
			}

			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"/root/sub"}, failed)
	})

	t.Run("link to a file is reported without descending", func(t *testing.T) {
		t.Parallel()

		root := evaledTempDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644))
		require.NoError(t, os.Symlink(filepath.Join(root, "file.txt"), filepath.Join(root, "link")))

		assert.ElementsMatch(t, []string{".", "file.txt", "link"}, walkSymlinkedPaths(t, root))
	})

	t.Run("link whose target cannot be described is handed to the callback", func(t *testing.T) {
		t.Parallel()

		root := evaledTempDir(t)
		target := filepath.Join(root, "dir")
		require.NoError(t, os.Mkdir(target, 0o755))
		require.NoError(t, os.Symlink(target, filepath.Join(root, "link")))

		fsys := evalFaultFS{faultFS: &faultFS{
			FS:     vfs.NewOSFS(),
			faults: map[string]string{faultStat: target},
		}}

		var failed []string

		err := vfs.WalkDirWithSymlinks(fsys, root, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				require.ErrorIs(t, err, errInjected)

				failed = append(failed, path)
			}

			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(root, "link")}, failed)
	})
}

func TestEvalSymlinksWalk(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/real/dir/file", []byte("x"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/real", "/linked"))

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "trailing separator", path: "/linked/dir/", want: "/real/dir"},
		{name: "dot component", path: "/linked/./dir/file", want: "/real/dir/file"},
		{name: "dot-dot component", path: "/linked/dir/../dir/file", want: "/real/dir/file"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := vfs.EvalSymlinks(fsys, filepath.FromSlash(tc.path))
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash(tc.want), got)
		})
	}

	t.Run("file used as a directory is ENOTDIR", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.EvalSymlinks(fsys, "/real/dir/file/child")
		require.ErrorIs(t, err, syscall.ENOTDIR)
	})

	t.Run("link cycle is refused", func(t *testing.T) {
		t.Parallel()

		cyclic := vfs.NewMemMapFS()
		require.NoError(t, vfs.Symlink(cyclic, "/b", "/a"))
		require.NoError(t, vfs.Symlink(cyclic, "/a", "/b"))

		_, err := vfs.EvalSymlinks(cyclic, "/a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many links")
	})

	t.Run("relative link at the top of a relative path", func(t *testing.T) {
		t.Parallel()

		relative := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(relative, "target", []byte("x"), 0o644))
		require.NoError(t, vfs.Symlink(relative, "target", "link"))

		got, err := vfs.EvalSymlinks(relative, "link")
		require.NoError(t, err)
		assert.Equal(t, "target", got)
	})

	t.Run("link that cannot be read back is an error", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(base, "/target", []byte("x"), 0o644))
		require.NoError(t, vfs.Symlink(base, "/target", "/link"))

		_, err := vfs.EvalSymlinks(noReadlinkFS{FS: base}, "/link")
		require.ErrorIs(t, err, afero.ErrNoSymlink)
	})
}

func TestValidateResolvedSymlinkTargetUnresolvableRoot(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/root/target", []byte("x"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/root/target", "/root/link"))

	err := vfs.ValidateResolvedSymlinkTarget(fsys, "/missing", "/root/link")
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.NotErrorIs(t, err, vfs.ErrSymlinkEscapes)
}

// newWalkTree returns an in-memory tree holding /root/a.txt,
// /root/sub/inner.txt and /root/z.txt, which walk in that order.
func newWalkTree(t *testing.T) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()
	for _, path := range []string{"/root/a.txt", "/root/sub/inner.txt", "/root/z.txt"} {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(path), 0o644))
	}

	return fsys
}

// evalFaultFS is a faultFS that resolves symlinks natively, so a walk can
// reach a link's target without describing it through the faulty Stat.
type evalFaultFS struct {
	*faultFS
}

func (evalFaultFS) EvalSymlinksIfPossible(name string) (string, bool, error) {
	resolved, err := filepath.EvalSymlinks(name)

	return resolved, true, err
}

// noReadlinkFS can tell a symbolic link apart but cannot read one back.
type noReadlinkFS struct {
	vfs.FS
}

func (fsys noReadlinkFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	info, err := vfs.Lstat(fsys.FS, name)

	return info, true, err
}
