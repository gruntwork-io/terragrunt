package vfs_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "out.txt")

	require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte("hello"), 0o600))

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(contents))

	info, err := fsys.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestWriteFileAtomicCreatesParentDirs confirms a destination under a directory
// that does not exist yet is written rather than failing on the scratch file.
func TestWriteFileAtomicCreatesParentDirs(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "generated", "nested", "out.txt")

	require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte("hello"), 0o600))

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(contents))
}

// TestWriteFileAtomicReplacesSymlink confirms a symlink at the destination is
// replaced rather than followed, so the content goes to the path that was named.
func TestWriteFileAtomicReplacesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))

	path := filepath.Join(dir, "out.txt")
	require.NoError(t, os.Symlink(target, path))

	require.NoError(t, vfs.WriteFileAtomic(vfs.NewOSFS(), path, []byte("updated"), 0o600))

	contents, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "original", string(contents), "the symlink target was written to")

	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "the symlink is still in place")
}

// TestWriteFileAtomicTightensExistingMode confirms the destination takes perm
// rather than keeping the mode it already had, which a plain create cannot do for
// a file that exists.
func TestWriteFileAtomicTightensExistingMode(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "out.txt")

	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))

	require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte("new"), 0o600))

	info, err := fsys.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestStreamFileAtomicLeavesPreviousFileOnFailure(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte("first"), 0o600))

	sentinel := errors.New("write failed")
	err := vfs.StreamFileAtomic(fsys, path, 0o600, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial"); err != nil {
			return err
		}

		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.Equal(t, "first", string(contents))

	entries, err := vfs.ReadDir(fsys, dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a scratch file was left behind")
}

// TestStreamFileAtomicConcurrentWithRacing pins that the destination holds one
// writer's content rather than a mixture when several target it at once.
func TestStreamFileAtomicConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		size    = 256 << 10
	)

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "out.txt")

	bodies := make([]string, writers)
	for i := range bodies {
		bodies[i] = strconv.Itoa(i) + string(make([]byte, size))
	}

	group, _ := errgroup.WithContext(t.Context())

	for i := range writers {
		group.Go(func() error {
			return vfs.StreamFileAtomic(fsys, path, 0o600, func(w io.Writer) error {
				_, err := io.WriteString(w, bodies[i])

				return err
			})
		})
	}

	require.NoError(t, group.Wait())

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.Contains(t, bodies, string(contents), "the file is a mixture of writers")
}

// TestWriteFileAtomicNameAtComponentLimit covers a destination whose own name
// fills the per-component limit, where a scratch name built from it untrimmed
// would be too long for the directory that just accepted the destination.
func TestWriteFileAtomicNameAtComponentLimit(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows caps the whole path, not the component, so a maximal name proves nothing here")
	}

	const nameMax = 255

	testCases := []struct {
		name string
		base string
	}{
		{
			name: "single-byte runes",
			base: strings.Repeat("a", nameMax),
		},
		{
			// The leading byte offsets the runes so the trim falls inside one
			// of them, which is the cut macOS rejects the name for.
			name: "multi-byte runes",
			base: "a" + strings.Repeat("\u00e9", (nameMax-1)/2),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewOSFS()
			dir := t.TempDir()
			path := filepath.Join(dir, tc.base)

			require.Len(t, tc.base, nameMax)
			require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte("hello"), 0o600))

			contents, err := vfs.ReadFile(fsys, path)
			require.NoError(t, err)
			assert.Equal(t, "hello", string(contents))

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 1, "the scratch file must not outlive the write")
		})
	}
}
