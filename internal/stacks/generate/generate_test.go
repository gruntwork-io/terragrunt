package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanonicalStackFilePathKeepsSymlinkDirectory reproduces the regression where a symlinked
// terragrunt.stack.hcl had its source directory rewritten to the symlink target's directory,
// so read_terragrunt_config and other relative reads looked in the wrong folder. The directory
// portion must be canonicalised, but the stack file must stay anchored to the directory it
// lives in rather than resolving through the symlink to its target's directory.
func TestCanonicalStackFilePathKeepsSymlinkDirectory(t *testing.T) {
	t.Parallel()

	// Resolve the temp root up front so the assertion is not confused by e.g. /var -> /private/var.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// The real stack file lives in a "source" directory that is NOT where terragrunt is invoked.
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, config.DefaultStackFile), []byte("# stack\n"), 0644))

	// The "live" directory holds a symlink to the real stack file plus the sibling file the stack
	// would read relatively. read_terragrunt_config must resolve against liveDir, not sourceDir.
	liveDir := filepath.Join(base, "live")
	require.NoError(t, os.MkdirAll(liveDir, 0755))

	symlink := filepath.Join(liveDir, config.DefaultStackFile)
	require.NoError(t, os.Symlink(filepath.Join(sourceDir, config.DefaultStackFile), symlink))

	got, err := canonicalStackFilePath(liveDir, base)
	require.NoError(t, err)

	// The generated source dir is filepath.Dir of the returned stack-file path. It must be the
	// directory the symlink lives in, not the symlink target's directory.
	assert.Equal(t, liveDir, filepath.Dir(got),
		"symlinked stack file should keep its own directory as the source dir")
	assert.NotEqual(t, sourceDir, filepath.Dir(got),
		"symlinked stack file must not be rewritten to the symlink target's directory")
}

// TestCanonicalStackFilePathResolvesDirectory confirms the directory portion is still
// symlink-resolved, so topology keys stay consistent with the canonicalised working dir.
func TestCanonicalStackFilePathResolvesDirectory(t *testing.T) {
	t.Parallel()

	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realDir := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(realDir, 0755))

	// A symlinked directory should resolve to its target for a consistent, deduplicable key.
	linkDir := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realDir, linkDir))

	got, err := canonicalStackFilePath(linkDir, base)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(realDir, config.DefaultStackFile), got)
	// Sanity check against the raw util helper on a non-symlinked directory.
	direct, err := util.CanonicalResolvedPath(realDir, base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(direct, config.DefaultStackFile), got)
}
