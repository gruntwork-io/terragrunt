package cliconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveReplacesSymlink confirms a symlink at the config path is replaced
// rather than followed, so Save writes to the path it was given.
func TestSaveReplacesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.hcl")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))

	configPath := filepath.Join(dir, "config.hcl")
	require.NoError(t, os.Symlink(target, configPath))

	cfg := cliconfig.NewConfig(vfs.NewOSFS()).WithCredentials([]cliconfig.ConfigCredentials{
		{Name: "registry.opentofu.org", Token: "example-token"},
	})

	require.NoError(t, cfg.Save(configPath))

	contents, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "original", string(contents), "the symlink target was written to")

	info, err := os.Lstat(configPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "the symlink is still in place")

	written, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(written), "example-token")

	info, err = os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
