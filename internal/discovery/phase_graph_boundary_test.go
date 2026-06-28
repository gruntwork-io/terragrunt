package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestDiscoveryDependentBoundary_InPlace verifies that --discovery-boundary
// (WithBoundary) constrains the upstream dependent walk to a directory subtree.
//
// Layout (git root = tmpDir, working dir = proj):
//
//	proj/base      <- target
//	proj/app       <- depends on ../base       (in boundary)
//	consumer       <- depends on ../proj/base   (sibling subtree, out of boundary)
//
// The filesystem phase only discovers units under the working dir (proj), so the
// out-of-boundary consumer can only ever be reached by the upstream walk. Bounding
// that walk to proj therefore excludes it; without a boundary it is included.
func TestDiscoveryDependentBoundary_InPlace(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	projDir := filepath.Join(tmpDir, "proj")
	baseDir := filepath.Join(projDir, "base")
	appDir := filepath.Join(projDir, "app")
	consumerDir := filepath.Join(tmpDir, "consumer")

	for _, dir := range []string{baseDir, appDir, consumerDir} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
dependency "base" {
  config_path = "../base"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(consumerDir, "terragrunt.hcl"), []byte(`
dependency "base" {
  config_path = "../proj/base"
}
`), 0o644))

	run := func(boundary string) []string {
		opts := options.NewTerragruntOptions()
		opts.WorkingDir = projDir
		opts.RootWorkingDir = projDir

		filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{`...{` + baseDir + `}`})
		require.NoError(t, err)

		d := discovery.NewDiscovery(projDir).
			WithExec(memGitTopLevelExec(t, tmpDir)).
			WithFilters(filters)

		if boundary != "" {
			d = d.WithBoundary(boundary)
		}

		configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
		require.NoError(t, err)

		return configs.Filter(component.UnitKind).Paths()
	}

	// Control: no boundary, the walk reaches the git root and finds the sibling consumer.
	control := run("")
	assert.Contains(t, control, baseDir, "target should be discovered")
	assert.Contains(t, control, appDir, "in-boundary dependent should be discovered")
	assert.Contains(t, control, consumerDir, "without a boundary the out-of-boundary dependent is included")

	// Bounded to proj: the out-of-boundary consumer is excluded, the in-boundary app remains.
	bounded := run(projDir)
	assert.Contains(t, bounded, baseDir, "target should still be discovered")
	assert.Contains(t, bounded, appDir, "in-boundary dependent should still be discovered")
	assert.NotContains(t, bounded, consumerDir, "boundary should exclude the out-of-boundary dependent")
}

// TestDiscoveryDependentBoundary_Worktree verifies that --discovery-boundary
// constrains dependent discovery for Git-based filters, which run inside a full
// worktree checkout. The boundary (a real-tree path) is translated into the
// worktree before the upstream walk.
//
// Layout (git root = tmpDir): proj/base changes between HEAD~1 and HEAD; proj/app
// depends on it (in boundary) and consumer depends on it (out of boundary). With
// boundary = proj, the consumer is excluded; without it, it is included.
func TestDiscoveryDependentBoundary_Worktree(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	createUnit(t, tmpDir, filepath.Join("proj", "base"), `# base`)
	createUnit(t, tmpDir, filepath.Join("proj", "app"), `
dependency "base" {
  config_path = "../base"
}
`)
	createUnit(t, tmpDir, "consumer", `
dependency "base" {
  config_path = "../proj/base"
}
`)
	commitChanges(t, runner, "initial commit")

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "proj", "base", "terragrunt.hcl"),
		[]byte("locals {\n  changed = true\n}\n"), 0o644))
	commitChanges(t, runner, "change base")

	sep := string(filepath.Separator)

	// Control: no boundary, the walk covers the whole worktree and finds the consumer.
	control := runBoundaryWorktreeDiscovery(t, tmpDir, "")
	assert.True(t, anyHasSuffix(control, sep+"consumer"),
		"without a boundary the out-of-boundary dependent is included: %v", control)

	// Bounded to proj: the consumer is excluded while the in-boundary app remains.
	bounded := runBoundaryWorktreeDiscovery(t, tmpDir, filepath.Join(tmpDir, "proj"))
	assert.True(t, anyHasSuffix(bounded, sep+"app"),
		"in-boundary dependent should still be discovered: %v", bounded)
	assert.False(t, anyHasSuffix(bounded, sep+"consumer"),
		"boundary should exclude the out-of-boundary dependent: %v", bounded)
}

// runBoundaryWorktreeDiscovery runs a git-filter discovery for `...[HEAD~1...HEAD]`
// from tmpDir, optionally constrained by boundary, and returns the unit paths.
func runBoundaryWorktreeDiscovery(t *testing.T, tmpDir, boundary string) []string {
	t.Helper()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"...[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.WithoutCancel(t.Context()), l))
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithWorktrees(w).
		WithFilters(filters)

	if boundary != "" {
		d = d.WithBoundary(boundary)
	}

	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	return components.Filter(component.UnitKind).Paths()
}

func anyHasSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}

	return false
}
