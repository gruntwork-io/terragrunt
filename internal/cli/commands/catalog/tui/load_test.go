package tui_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCatalogTempPathResolvesSymlinkedTmpDir verifies the clone cache dir is
// anchored at a symlink-resolved temp dir. macOS reports os.TempDir() as a
// /var/folders symlink to /private/var/folders; an unresolved root makes
// discovery's filepath.Rel emit a "../" traversal that go-getter rejects.
func TestCatalogTempPathResolvesSymlinkedTmpDir(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realTmp := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(realTmp, 0o755))

	// linkTmp -> realTmp stands in for /var/folders -> /private/var/folders.
	linkTmp := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realTmp, linkTmp))

	t.Setenv("TMPDIR", linkTmp)

	got := tui.CatalogTempPath("github.com/gruntwork-io/terragrunt-scale-catalog")

	// The clone dir must sit directly under the resolved temp dir, with no
	// symlink component left for filepath.Rel to trip over later.
	assert.Equal(t, realTmp, filepath.Dir(got),
		"CatalogTempPath should anchor the clone at the symlink-resolved temp dir")
}
