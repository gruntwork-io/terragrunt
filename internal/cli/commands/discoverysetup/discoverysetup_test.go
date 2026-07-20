package discoverysetup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreesNoGitFilters(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	tmpDir := helpers.TmpDirWOSymlinks(t)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir

	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, venv.OSVenv(), opts, d)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, cleanup)

	cleanup(t.Context())
}

func TestWorktreesCreationFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	// A plain directory with no git repository makes worktree creation fail.
	tmpDir := helpers.TmpDirWOSymlinks(t)

	filters, err := filter.ParseFilterQueries(l, []string{"[main...HEAD]"})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.Filters = filters

	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
		Filters:    filters,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, venv.OSVenv(), opts, d)
	require.Error(t, err)
	assert.Same(t, d, got, "failed setup must not attach worktrees to the discovery")
	require.NotNil(t, cleanup)

	cleanup(t.Context())
}

func TestWorktreesStackGenerationFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	tmpDir := helpers.TmpDirWOSymlinks(t)
	runner := helpers.InitTestGitRunner(t, tmpDir)

	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("initial\n"), 0o644),
	)
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// A stack file that fails to parse makes stack generation inside the
	// worktrees fail after the worktrees themselves were created.
	stackFile := filepath.Join(tmpDir, "terragrunt.stack.hcl")
	require.NoError(t, os.WriteFile(stackFile, []byte(`unit "broken" {`), 0o644))

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Add broken stack"))

	filters, err := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.Filters = filters

	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
		Filters:    filters,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, venv.OSVenv(), opts, d)
	require.Error(t, err)
	assert.Same(t, d, got, "failed setup must not attach worktrees to the discovery")
	require.NotNil(t, cleanup)

	// Cleanup must still remove the worktrees created before generation failed.
	cleanup(t.Context())
}
