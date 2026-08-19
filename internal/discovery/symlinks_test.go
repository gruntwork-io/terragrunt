package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoverySymlinksExperiment pins which walk the discovery filesystem phase
// picks: with the symlinks experiment enabled a symlinked unit directory is walked
// as though it were real and surfaces as its own unit, and without it the link is
// reported as a plain entry and never descended into.
//
// This case stays on the OS filesystem because the in-memory filesystem keeps
// symlinks in a side table that directory reads do not surface, which would leave
// the symlink-following walk nothing to follow.
func TestDiscoverySymlinksExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping: creating symlinks on Windows requires elevated privileges")
	}

	root := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv()

	unitDir := filepath.Join(root, "a")
	require.NoError(t, v.FS.MkdirAll(unitDir, 0755))
	require.NoError(
		t,
		vfs.WriteFile(v.FS, filepath.Join(unitDir, "terragrunt.hcl"), []byte(``), 0644),
	)

	linkDir := filepath.Join(root, "b")
	require.NoError(t, vfs.Symlink(v.FS, unitDir, linkDir))

	discover := func(t *testing.T, experiments ...string) []string {
		t.Helper()

		opts := options.NewTerragruntOptions()
		opts.WorkingDir = root
		opts.RootWorkingDir = root

		for _, name := range experiments {
			require.NoError(t, opts.Experiments.EnableExperiment(name))
		}

		components, err := discovery.NewDiscovery(root).
			Discover(t.Context(), logger.CreateLogger(), v, opts)
		require.NoError(t, err)

		return components.Filter(component.UnitKind).Paths()
	}

	t.Run("experiment disabled", func(t *testing.T) {
		t.Parallel()

		assert.ElementsMatch(t, []string{unitDir}, discover(t))
	})

	t.Run("experiment enabled", func(t *testing.T) {
		t.Parallel()

		assert.ElementsMatch(t, []string{unitDir, linkDir}, discover(t, experiment.Symlinks))
	})
}
