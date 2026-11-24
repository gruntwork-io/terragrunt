package vfs_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathResolverResolvesSymlinkAlias(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/root/real/sub", 0o755))
	require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

	r := vfs.NewPathResolver(fs)

	assert.Equal(t, filepath.FromSlash("/root/real/sub"), r.Resolve("/root/link/sub"))
	assert.Equal(t, filepath.FromSlash("/root/real/sub"), r.Resolve("/root/real/sub"))
}

func TestPathResolverResolvesMissingPathUnderSymlink(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/root/real", 0o755))
	require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

	r := vfs.NewPathResolver(fs)

	assert.Equal(t, filepath.FromSlash("/root/real/missing"), r.Resolve("/root/link/missing"))
}

func TestPathResolverKeepsFirstResolution(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/root/first/sub", 0o755))
	require.NoError(t, fs.MkdirAll("/root/second/sub", 0o755))
	require.NoError(t, vfs.Symlink(fs, "/root/first", "/root/link"))

	r := vfs.NewPathResolver(fs)

	assert.Equal(t, filepath.FromSlash("/root/first/sub"), r.Resolve("/root/link/sub"))

	require.NoError(t, fs.Remove("/root/link"))
	require.NoError(t, vfs.Symlink(fs, "/root/second", "/root/link"))

	assert.Equal(t, filepath.FromSlash("/root/first/sub"), r.Resolve("/root/link/sub"))
	assert.Equal(t, filepath.FromSlash("/root/first/sub"), r.Resolve("/root/link/./sub"))
	assert.Equal(t, filepath.FromSlash("/root/second/sub"), vfs.NewPathResolver(fs).Resolve("/root/link/sub"))
}

func TestPathResolverConcurrentResolveWithRacing(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/root/real/sub", 0o755))
	require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

	r := vfs.NewPathResolver(fs)

	const goroutines = 32

	results := make([]string, goroutines)

	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Go(func() {
			results[i] = r.Resolve("/root/link/sub")
		})
	}

	wg.Wait()

	for _, got := range results {
		assert.Equal(t, filepath.FromSlash("/root/real/sub"), got)
	}
}
