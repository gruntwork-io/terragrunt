package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
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

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
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

// TestDiscoveryDependentsWalkStopsAtSymlinkedDirectory pins that the upstream
// dependents walk does not descend into a directory it climbs to when that
// directory is a symlink, even though the walk below it runs in parallel.
//
// The working directory is reached through a symlink to its real parent, and a
// sibling of the working directory depends on the target. The sibling is found
// only once the climb passes above the link, and then under its real path.
func TestDiscoveryDependentsWalkStopsAtSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping: creating symlinks on Windows requires elevated privileges")
	}

	root := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv()

	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "link")

	writeUnit := func(dir, body string) {
		require.NoError(t, v.FS.MkdirAll(dir, 0755))
		require.NoError(
			t,
			vfs.WriteFile(v.FS, filepath.Join(dir, "terragrunt.hcl"), []byte(body), 0644),
		)
	}

	writeUnit(filepath.Join(realDir, "root", "vpc"), "")
	writeUnit(
		filepath.Join(realDir, "sibling"),
		"dependency \"vpc\" {\n  config_path = \"../root/vpc\"\n}\n",
	)
	require.NoError(t, vfs.Symlink(v.FS, realDir, linkDir))

	workingDir := filepath.Join(linkDir, "root")
	target := filepath.Join(workingDir, "vpc")

	discover := func(t *testing.T, gitRoot string) []string {
		t.Helper()

		l := logger.CreateLogger()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.WorkingDir = workingDir
		opts.RootWorkingDir = workingDir

		filters, err := filter.ParseFilterQueries(l, []string{"...{" + target + "}"})
		require.NoError(t, err)

		components, err := discovery.NewDiscovery(workingDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
			WithGitRoot(gitRoot).
			WithFilters(filters).
			Discover(t.Context(), l, v, opts)
		require.NoError(t, err)

		return components.Filter(component.UnitKind).Paths()
	}

	t.Run("boundary above the link", func(t *testing.T) {
		t.Parallel()

		assert.ElementsMatch(
			t,
			[]string{target, filepath.Join(realDir, "sibling")},
			discover(t, root),
		)
	})

	t.Run("boundary at the link", func(t *testing.T) {
		t.Parallel()

		assert.ElementsMatch(t, []string{target}, discover(t, linkDir))
	})
}
