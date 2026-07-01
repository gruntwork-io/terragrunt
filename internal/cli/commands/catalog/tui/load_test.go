package tui_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateCatalogTempPathResolvesSymlinkedTmpDir verifies the clone dir is
// anchored at a symlink-resolved temp dir. macOS reports os.TempDir() as a
// /var/folders symlink to /private/var/folders; an unresolved root makes
// discovery's filepath.Rel emit a "../" traversal that go-getter rejects.
func TestCreateCatalogTempPathResolvesSymlinkedTmpDir(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realTmp := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(realTmp, 0o755))

	// linkTmp -> realTmp stands in for /var/folders -> /private/var/folders.
	linkTmp := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realTmp, linkTmp))

	t.Setenv("TMPDIR", linkTmp)

	got, err := tui.CreateCatalogTempPath(vfs.NewOSFS())
	require.NoError(t, err)

	// The clone dir must sit directly under the resolved temp dir, with no
	// symlink component left for filepath.Rel to trip over later.
	assert.Equal(t, realTmp, filepath.Dir(got),
		"CreateCatalogTempPath should anchor the clone at the symlink-resolved temp dir")
}

func TestCreateCatalogTempPathUsesFreshDirectory(t *testing.T) {
	t.Parallel()

	first, err := tui.CreateCatalogTempPath(vfs.NewOSFS())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(first))
	})

	second, err := tui.CreateCatalogTempPath(vfs.NewOSFS())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(second))
	})

	assert.NotEqual(t, first, second)
	assert.DirExists(t, first)
	assert.DirExists(t, second)
}
