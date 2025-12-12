package discovery_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// Test that WithGraphTarget retains the target and all dependents.
func TestDiscoveryWithGraphTarget_RetainsTargetAndDependents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Initialize a git repository in the temp directory so dependent discovery bounds traversal to the repo root.
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Create dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	require.NoError(t, os.MkdirAll(appDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
dependency "db" {
  config_path = "../db"
}
`), 0o644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoverDependencies().
		WithGraphTarget(vpcDir)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir, appDir}, paths)
}

// Test parity: experiment ON via filter queries vs graphTarget marker path
func TestDiscoveryGraphTarget_ParityWithFilterQueries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize a git repository in the temp directory so dependent discovery bounds traversal to the repo root.
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Create dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	require.NoError(t, os.MkdirAll(appDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
dependency "db" {
  config_path = "../db"
}
`), 0o644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Path A: filter queries (experiment ON equivalent)
	filters, err := filter.ParseFilterQueries([]string{`...{` + vpcDir + `}`})
	require.NoError(t, err)

	configsA, err := discovery.NewDiscovery(tmpDir).
		WithDiscoverDependencies().
		WithFilters(filters).
		Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Path B: graph target marker
	configsB, err := discovery.NewDiscovery(tmpDir).
		WithDiscoverDependencies().
		WithGraphTarget(vpcDir).
		Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	assert.ElementsMatch(t, configsA.Filter(component.UnitKind).Paths(), configsB.Filter(component.UnitKind).Paths())
}
