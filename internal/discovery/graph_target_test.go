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
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// Test that WithGraphTarget retains the target and all dependents.
func TestDiscoveryWithGraphTarget_RetainsTargetAndDependents(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	depsFilters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithFilters(depsFilters).
		WithGraphTarget(vpcDir)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir, appDir}, paths)
}

// Test parity: experiment ON via filter queries vs graphTarget marker path
func TestDiscoveryGraphTarget_ParityWithFilterQueries(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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
	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{`...{` + vpcDir + `}`})
	require.NoError(t, err)

	depsFilters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"{./**}..."})
	require.NoError(t, err)

	configsA, err := discovery.NewDiscovery(tmpDir).
		WithFilters(depsFilters).
		WithFilters(filters).
		Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Path B: graph target marker
	configsB, err := discovery.NewDiscovery(tmpDir).
		WithFilters(depsFilters).
		WithGraphTarget(vpcDir).
		Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	assert.ElementsMatch(t, configsA.Filter(component.UnitKind).Paths(), configsB.Filter(component.UnitKind).Paths())
}

// Test that graph target with no dependents returns only the target.
func TestDiscoveryWithGraphTarget_NoDependents(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize a git repository
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Create standalone units (no dependencies between them)
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	require.NoError(t, os.MkdirAll(appDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(``), 0o644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithRelationships().
		WithGraphTarget(vpcDir)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	// Should only return the target since no one depends on it
	assert.ElementsMatch(t, []string{vpcDir}, paths)
}

// Test that WithOptions interface assertion works for GraphTarget.
func TestDiscoveryWithOptions_GraphTarget(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize a git repository
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Create dependency chain: vpc -> db
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(dbDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0o644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create an option that implements GraphTarget() interface
	graphTargetOpt := &mockGraphTargetOption{target: vpcDir}

	d := discovery.NewDiscovery(tmpDir).
		WithRelationships().
		WithOptions(graphTargetOpt)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir}, paths)
}

// mockGraphTargetOption implements the GraphTarget() interface for testing.
type mockGraphTargetOption struct {
	target string
}

func (m *mockGraphTargetOption) GraphTarget() string {
	return m.target
}
