package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overlayBase builds a base filesystem for the overlay tests and returns
// the directory its fixture tree sits under.
type overlayBase struct {
	new  func(t *testing.T) (vfs.FS, string)
	name string
}

func overlayBases() []overlayBase {
	return []overlayBase{
		{
			name: "mem base",
			new: func(t *testing.T) (vfs.FS, string) {
				t.Helper()

				return vfs.NewMemMapFS(), filepath.FromSlash("/base")
			},
		},
		{
			name: "os base",
			new: func(t *testing.T) (vfs.FS, string) {
				t.Helper()

				return vfs.NewOSFS(), t.TempDir()
			},
		},
	}
}

// newOverlay returns an overlay over a base holding file.txt, dir/a.txt and
// dir/sub/deep.txt under root, with the base and layer it was built from.
func newOverlay(t *testing.T, b overlayBase) (overlay, base, layer vfs.FS, root string) {
	t.Helper()

	base, root = b.new(t)

	require.NoError(t, vfs.WriteFile(base, filepath.Join(root, "file.txt"), []byte("base"), 0o644))
	require.NoError(t, vfs.WriteFile(base, filepath.Join(root, "dir", "a.txt"), []byte("a"), 0o644))
	require.NoError(t, vfs.WriteFile(base, filepath.Join(root, "dir", "sub", "deep.txt"), []byte("deep"), 0o644))

	layer = vfs.NewMemMapFS()

	return vfs.NewOverlayFS(base, layer), base, layer, root
}

func entryNames(t *testing.T, fsys vfs.FS, dir string) []string {
	t.Helper()

	entries, err := vfs.ReadDir(fsys, dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func symlinkOrSkip(t *testing.T, fsys vfs.FS, target, link string) {
	t.Helper()

	if err := vfs.Symlink(fsys, target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
}

func TestNewOverlayFS(t *testing.T) {
	t.Parallel()

	for _, b := range overlayBases() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			t.Run("reads what base holds", func(t *testing.T) {
				t.Parallel()

				overlay, _, _, root := newOverlay(t, b)

				got, err := vfs.ReadFile(overlay, filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, []byte("base"), got)

				assert.Equal(t, []string{"a.txt", "sub"}, entryNames(t, overlay, filepath.Join(root, "dir")))
				assert.True(t, vfs.IsDir(overlay, filepath.Join(root, "dir", "sub")))
				assert.False(t, vfs.Exists(overlay, filepath.Join(root, "missing")))
			})

			t.Run("is not the OS filesystem", func(t *testing.T) {
				t.Parallel()

				overlay, _, _, _ := newOverlay(t, b)

				assert.False(t, vfs.IsOSFS(overlay))
			})

			t.Run("writes a new file into the layer", func(t *testing.T) {
				t.Parallel()

				overlay, base, layer, root := newOverlay(t, b)
				added := filepath.Join(root, "dir", "sub", "added.txt")

				require.NoError(t, vfs.WriteFile(overlay, added, []byte("added"), 0o644))

				got, err := vfs.ReadFile(overlay, added)
				require.NoError(t, err)
				assert.Equal(t, []byte("added"), got)

				got, err = vfs.ReadFile(layer, added)
				require.NoError(t, err)
				assert.Equal(t, []byte("added"), got)

				_, err = base.Stat(added)
				require.ErrorIs(t, err, fs.ErrNotExist)
			})

			t.Run("copies a base file into the layer before changing it", func(t *testing.T) {
				t.Parallel()

				overlay, base, layer, root := newOverlay(t, b)
				file := filepath.Join(root, "file.txt")

				require.NoError(t, vfs.WriteFile(overlay, file, []byte("changed"), 0o644))

				got, err := vfs.ReadFile(overlay, file)
				require.NoError(t, err)
				assert.Equal(t, []byte("changed"), got)

				got, err = vfs.ReadFile(layer, file)
				require.NoError(t, err)
				assert.Equal(t, []byte("changed"), got)

				got, err = vfs.ReadFile(base, file)
				require.NoError(t, err)
				assert.Equal(t, []byte("base"), got)
			})

			t.Run("lists the union of both with the layer winning", func(t *testing.T) {
				t.Parallel()

				overlay, _, layer, root := newOverlay(t, b)
				dir := filepath.Join(root, "dir")

				require.NoError(t, vfs.WriteFile(layer, filepath.Join(dir, "b.txt"), []byte("b"), 0o644))
				require.NoError(t, vfs.WriteFile(layer, filepath.Join(dir, "a.txt"), []byte("layer a"), 0o644))

				assert.Equal(t, []string{"a.txt", "b.txt", "sub"}, entryNames(t, overlay, dir))

				got, err := vfs.ReadFile(overlay, filepath.Join(dir, "a.txt"))
				require.NoError(t, err)
				assert.Equal(t, []byte("layer a"), got)

				var walked []string

				require.NoError(t, vfs.WalkDir(overlay, dir, func(path string, _ fs.DirEntry, err error) error {
					if err != nil {
						return err
					}

					rel, err := filepath.Rel(dir, path)
					if err != nil {
						return err
					}

					walked = append(walked, filepath.ToSlash(rel))

					return nil
				}))

				assert.Equal(t, []string{".", "a.txt", "b.txt", "sub", "sub/deep.txt"}, walked)
			})

			t.Run("makes directories in the layer", func(t *testing.T) {
				t.Parallel()

				overlay, base, layer, root := newOverlay(t, b)
				fresh := filepath.Join(root, "fresh")

				require.ErrorIs(t, overlay.Mkdir(filepath.Join(root, "dir"), 0o755), fs.ErrExist)
				require.NoError(t, overlay.MkdirAll(filepath.Join(root, "dir"), 0o755))
				require.NoError(t, overlay.Mkdir(fresh, 0o755))

				assert.True(t, vfs.IsDir(overlay, fresh))
				assert.True(t, vfs.IsDir(layer, fresh))
				assert.False(t, vfs.Exists(base, fresh))
			})

			t.Run("refuses to remove or rename what base holds", func(t *testing.T) {
				t.Parallel()

				overlay, base, _, root := newOverlay(t, b)
				file := filepath.Join(root, "file.txt")
				dir := filepath.Join(root, "dir")

				require.ErrorIs(t, overlay.Remove(file), fs.ErrPermission)
				require.ErrorIs(t, overlay.RemoveAll(dir), fs.ErrPermission)
				require.ErrorIs(t, overlay.Rename(file, filepath.Join(root, "moved.txt")), fs.ErrPermission)

				require.NoError(t, vfs.WriteFile(overlay, file, []byte("changed"), 0o644))
				require.ErrorIs(t, overlay.Remove(file), fs.ErrPermission)

				got, err := vfs.ReadFile(overlay, file)
				require.NoError(t, err)
				assert.Equal(t, []byte("changed"), got)

				assert.True(t, vfs.Exists(base, file))
				assert.True(t, vfs.IsDir(base, dir))
			})

			t.Run("removes and renames what only the layer holds", func(t *testing.T) {
				t.Parallel()

				overlay, _, _, root := newOverlay(t, b)
				created := filepath.Join(root, "dir", "created.txt")
				moved := filepath.Join(root, "dir", "moved.txt")

				require.NoError(t, vfs.WriteFile(overlay, created, []byte("created"), 0o644))
				require.NoError(t, overlay.Rename(created, moved))

				got, err := vfs.ReadFile(overlay, moved)
				require.NoError(t, err)
				assert.Equal(t, []byte("created"), got)
				assert.False(t, vfs.Exists(overlay, created))

				require.NoError(t, overlay.Remove(moved))
				assert.False(t, vfs.Exists(overlay, moved))

				require.ErrorIs(t, overlay.Remove(moved), fs.ErrNotExist)

				tree := filepath.Join(root, "tree")
				require.NoError(t, vfs.WriteFile(overlay, filepath.Join(tree, "leaf.txt"), []byte("leaf"), 0o644))
				require.NoError(t, overlay.RemoveAll(tree))
				assert.False(t, vfs.Exists(overlay, tree))
			})

			t.Run("follows symlinks in base", func(t *testing.T) {
				t.Parallel()

				overlay, base, _, root := newOverlay(t, b)
				dir := filepath.Join(root, "dir")
				link := filepath.Join(root, "link")

				symlinkOrSkip(t, base, dir, link)

				info, err := vfs.Lstat(overlay, link)
				require.NoError(t, err)
				assert.Equal(t, os.ModeSymlink, info.Mode()&os.ModeSymlink)

				target, err := vfs.Readlink(overlay, link)
				require.NoError(t, err)
				assert.Equal(t, dir, target)

				got, err := vfs.ReadFile(overlay, filepath.Join(link, "a.txt"))
				require.NoError(t, err)
				assert.Equal(t, []byte("a"), got)

				want, err := vfs.EvalSymlinks(base, filepath.Join(link, "a.txt"))
				require.NoError(t, err)

				resolved, err := vfs.EvalSymlinks(overlay, filepath.Join(link, "a.txt"))
				require.NoError(t, err)
				assert.Equal(t, want, resolved)
			})

			t.Run("records symlinks it creates in the layer", func(t *testing.T) {
				t.Parallel()

				overlay, base, layer, root := newOverlay(t, b)
				target := filepath.Join(root, "layer-target.txt")
				link := filepath.Join(root, "layer-link")

				require.NoError(t, vfs.WriteFile(overlay, target, []byte("target"), 0o644))
				require.NoError(t, vfs.Symlink(overlay, target, link))

				got, err := vfs.Readlink(overlay, link)
				require.NoError(t, err)
				assert.Equal(t, target, got)

				got, err = vfs.Readlink(layer, link)
				require.NoError(t, err)
				assert.Equal(t, target, got)

				contents, err := vfs.ReadFile(overlay, link)
				require.NoError(t, err)
				assert.Equal(t, []byte("target"), contents)

				_, err = vfs.Lstat(base, link)
				require.ErrorIs(t, err, fs.ErrNotExist)
			})

			t.Run("locks on the layer", func(t *testing.T) {
				t.Parallel()

				overlay, base, _, root := newOverlay(t, b)
				name := filepath.Join(root, "lock")

				unlocker, err := vfs.Lock(overlay, name)
				require.NoError(t, err)

				_, acquired, err := vfs.TryLock(overlay, name)
				require.NoError(t, err)
				assert.False(t, acquired)

				require.NoError(t, unlocker.Unlock())

				unlocker, err = vfs.LockContext(t.Context(), overlay, name)
				require.NoError(t, err)
				require.NoError(t, unlocker.Unlock())

				assert.False(t, vfs.Exists(base, name))
			})
		})
	}
}
