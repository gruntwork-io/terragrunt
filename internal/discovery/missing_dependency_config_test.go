package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDeletedDependencyRepo commits a consumer unit depending on dep, then deletes
// dep in a second commit so HEAD~1...HEAD carries a dangling dependency reference.
func setupDeletedDependencyRepo(t *testing.T) string {
	t.Helper()

	tmpDir, runner := setupGitRepo(t)

	createUnit(t, tmpDir, "dep", `# dep`)
	createUnit(t, tmpDir, "consumer", `dependency "dep" {
  config_path = "../dep"
  mock_outputs = { name = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}`)

	commitChanges(t, runner, "Initial commit")

	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "dep")))
	commitChanges(t, runner, "Delete dep")

	return tmpDir
}

// runAllStyleDiscovery mirrors run --all discovery: worktrees for git filters,
// relationship discovery enabled, and parse errors NOT suppressed.
func runAllStyleDiscovery(
	t *testing.T,
	tmpDir string,
	queries []string,
) (component.Components, error) {
	t.Helper()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, queries)
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
			Cmd:        "plan",
		}).
		WithRelationships().
		WithWorktrees(w).
		WithFilters(filters)

	return d.Discover(t.Context(), l, venv.OSVenv(), opts)
}

// TestRelationshipPhase_GitFilterDeletedDependency pins that run --all-style discovery
// does not fail when a git-range filter's diff deletes a unit that a live unit still
// references via a dependency block. Regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6540.
func TestRelationshipPhase_GitFilterDeletedDependency(t *testing.T) {
	t.Parallel()

	tmpDir := setupDeletedDependencyRepo(t)

	// The union with the path filter pulls the live consumer in from the working
	// directory, whose dangling dependency the relationship phase then parses.
	components, err := runAllStyleDiscovery(t, tmpDir, []string{
		"...[HEAD~1...HEAD]... | ./**",
	})
	require.NoError(t, err)

	names := make([]string, 0, len(components))
	for _, c := range components {
		names = append(names, filepath.Base(c.Path()))
	}

	assert.Contains(t, names, "consumer")
}

// TestGraphPhase_GitFilterDeletedDependency pins the same crash for the graph phase,
// reached when the git-range filter expands through the dependency graph and the
// changed unit's dependency was deleted in the same diff.
func TestGraphPhase_GitFilterDeletedDependency(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	createUnit(t, tmpDir, "dep", `# dep`)
	createUnit(t, tmpDir, "consumer", `dependency "dep" {
  config_path = "../dep"
  mock_outputs = { name = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}`)

	commitChanges(t, runner, "Initial commit")

	// Modify consumer AND delete dep in the same diff so the graph expansion
	// traverses consumer's dangling dependency in the current worktree.
	createUnit(t, tmpDir, "consumer", `dependency "dep" {
  config_path = "../dep"
  mock_outputs = { name = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}
# modified`)
	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "dep")))
	commitChanges(t, runner, "Modify consumer, delete dep")

	components, err := runAllStyleDiscovery(t, tmpDir, []string{"[HEAD~1...HEAD]..."})
	require.NoError(t, err)

	assert.NotEmpty(t, components.Paths())
}

// TestRelationshipPhase_MissingDependencyConfigWithoutGitFilter pins that without
// git filter expressions a dangling dependency reference still fails discovery,
// preserving the long-standing run --all behavior for typo'd config_path values.
func TestRelationshipPhase_MissingDependencyConfigWithoutGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	createUnit(t, tmpDir, "consumer", `dependency "dep" {
  config_path = "../dep"
}`)
	// A second unit keeps the relationship phase's terminal tracker non-empty so
	// traversal actually descends into consumer's dangling dependency.
	createUnit(t, tmpDir, "other", `# other`)
	commitChanges(t, runner, "Initial commit")

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithRelationships()

	_, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.Error(t, err)

	var notFoundErr config.TerragruntConfigNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
}
