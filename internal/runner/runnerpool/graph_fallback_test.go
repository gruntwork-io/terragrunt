package runnerpool_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// Test that the runner-level fallback (WithGraphTarget) limits the stack to target + dependents,
// and that this matches the discovery-based graph filter behavior when the filter experiment is enabled.
func TestGraphFallbackMatchesFilterExperiment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := thlogger.CreateLogger()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	// Make tmpDir a git repository so graph root detection works consistently
	helpers.CreateGitRepo(t, tmpDir)

	// Create a simple dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	for _, dir := range []string{vpcDir, dbDir, appDir} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	// Minimal terragrunt.hcl files to express dependencies
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

	// Ensure each unit directory has at least one Terraform file to avoid being skipped during unit resolution.
	for _, dir := range []string{vpcDir, dbDir, appDir} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0o644))
	}

	// Path set we expect when targeting vpc: {vpc, db, app}
	expected := []string{vpcDir, dbDir, appDir}

	// Path 1: experiment ON, use discovery filter
	optsOn := options.NewTerragruntOptions()
	optsOn.WorkingDir = vpcDir
	optsOn.RootWorkingDir = tmpDir
	// Enable the filter-flag experiment
	require.NoError(t, optsOn.Experiments.EnableExperiment("filter-flag"))
	// Inject graph filter for dependents of target
	optsOn.FilterQueries = []string{`...{` + vpcDir + `}`}
	// Build runner
	runnerOn, err := runnerpool.Build(ctx, l, optsOn)
	require.NoError(t, err)
	// Collect unit paths
	onPaths := make([]string, 0, len(runnerOn.GetStack().Units))
	for _, u := range runnerOn.GetStack().Units {
		onPaths = append(onPaths, u.Path())
	}

	// Path 2: experiment OFF, use fallback option
	optsOff := options.NewTerragruntOptions()
	optsOff.WorkingDir = vpcDir
	optsOff.RootWorkingDir = tmpDir
	// No filter queries; rely on fallback graph target option
	runnerOff, err := runnerpool.Build(ctx, l, optsOff, discovery.WithGraphTarget(vpcDir))
	require.NoError(t, err)

	offPaths := make([]string, 0, len(runnerOff.GetStack().Units))
	for _, u := range runnerOff.GetStack().Units {
		offPaths = append(offPaths, u.Path())
	}

	// Both paths should include exactly target + dependents (order not guaranteed)
	assert.ElementsMatch(t, expected, onPaths)
	assert.ElementsMatch(t, expected, offPaths)
	assert.ElementsMatch(t, onPaths, offPaths)
}
