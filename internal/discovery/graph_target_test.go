package discovery_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// Test that WithGraphTarget retains the target and all dependents.
func TestDiscoveryWithGraphTarget_RetainsTargetAndDependents(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), memGitTopLevelVenv(t, tmpDir), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir, appDir}, paths)
}

// Test parity: experiment ON via filter queries vs graphTarget marker path
func TestDiscoveryGraphTarget_ParityWithFilterQueries(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	// Path A: via filter queries (`{./**}...`).
	filtersA, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"{./**}..."})
	require.NoError(t, err)

	configsA, err := discovery.NewDiscovery(tmpDir).
		WithFilters(filtersA).
		WithGraphTarget(vpcDir).
		Discover(t.Context(), logger.CreateLogger(), memGitTopLevelVenv(t, tmpDir), opts)
	require.NoError(t, err)

	// Path B: via graphTarget marker path alone.
	configsB, err := discovery.NewDiscovery(tmpDir).
		WithRelationships().
		WithGraphTarget(vpcDir).
		Discover(t.Context(), logger.CreateLogger(), memGitTopLevelVenv(t, tmpDir), opts)
	require.NoError(t, err)

	assert.ElementsMatch(t, configsA.Filter(component.UnitKind).Paths(), configsB.Filter(component.UnitKind).Paths())
}

// Test that graph target with no dependents returns only the target.
func TestDiscoveryWithGraphTarget_NoDependents(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), memGitTopLevelVenv(t, tmpDir), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	// Should only return the target since no one depends on it
	assert.ElementsMatch(t, []string{vpcDir}, paths)
}

// Test that WithOptions interface assertion works for GraphTarget.
func TestDiscoveryWithOptions_GraphTarget(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), memGitTopLevelVenv(t, tmpDir), opts)
	require.NoError(t, err)

	paths := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir}, paths)
}

// memGitTopLevelVenv returns a venv.Venv whose Exec answers
// `git rev-parse --show-toplevel` with the supplied repoRoot. Any other
// invocation fails the test so a regression that fires unexpected git
// subcommands is caught here.
func memGitTopLevelVenv(t *testing.T, repoRoot string) *venv.Venv {
	t.Helper()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "git" && len(inv.Args) == 2 && inv.Args[0] == "rev-parse" && inv.Args[1] == "--show-toplevel" {
			return vexec.Result{Stdout: []byte(repoRoot + "\n")}
		}

		assert.Fail(t, "unexpected git invocation", "name=%q args=%v", inv.Name, inv.Args)

		return vexec.Result{ExitCode: 1}
	})

	return &venv.Venv{
		Exec:    exec,
		Env:     map[string]string{},
		Writers: writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}

// mockGraphTargetOption implements the GraphTarget() interface for testing.
type mockGraphTargetOption struct {
	target string
}

func (m *mockGraphTargetOption) GraphTarget() string {
	return m.target
}
