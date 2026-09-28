package discoverysetup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreesNoGitFilters(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	tmpDir := helpers.TmpDirWOSymlinks(t)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir

	d, err := discovery.NewForDiscoveryCommand(l, v.FS, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, v, opts, d)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, cleanup)

	cleanup(t.Context())
}

func TestWorktreesCreationFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	// A plain directory with no git repository makes worktree creation fail.
	tmpDir := helpers.TmpDirWOSymlinks(t)

	filters, err := filter.ParseFilterQueries(l, []string{"[main...HEAD]"})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.Filters = filters

	d, err := discovery.NewForDiscoveryCommand(l, v.FS, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
		Filters:    filters,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, v, opts, d)
	require.Error(t, err)
	assert.Same(t, d, got, "failed setup must not attach worktrees to the discovery")
	require.NotNil(t, cleanup)

	cleanup(t.Context())
}

func TestWorktreesStackGenerationFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
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

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.Filters = filters

	d, err := discovery.NewForDiscoveryCommand(l, v.FS, &discovery.DiscoveryCommandOptions{
		WorkingDir: tmpDir,
		Filters:    filters,
	})
	require.NoError(t, err)

	got, cleanup, err := discoverysetup.Worktrees(t.Context(), l, v, opts, d)
	require.Error(t, err)
	assert.Same(t, d, got, "failed setup must not attach worktrees to the discovery")
	require.NotNil(t, cleanup)

	// Cleanup must still remove the worktrees created before generation failed.
	cleanup(t.Context())
}

func TestFilteredPathsOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		queries []string
		want    bool
	}{
		{
			name:    "a Git expression alone",
			queries: []string{"[main...HEAD]"},
			want:    true,
		},
		{
			name:    "several Git expressions",
			queries: []string{"[main...HEAD]", "[HEAD~1...HEAD]"},
			want:    true,
		},
		{
			// The second query reaches the worktree discovery as well, and
			// names a component the Git expression never mentions.
			name:    "a Git expression beside a path query",
			queries: []string{"[main...HEAD]", "stable"},
			want:    false,
		},
		{
			name:    "a query that has to know what a unit reads",
			queries: []string{"[main...HEAD]", "reading=root.hcl"},
			want:    false,
		},
		{
			// The whole query holds a Git expression, so nothing extra reaches
			// the worktree discovery.
			name:    "a path query joined to a Git expression",
			queries: []string{"[main...HEAD] | stable"},
			want:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			filters, err := filter.ParseFilterQueries(l, tc.queries)
			require.NoError(t, err)

			d, err := discovery.NewForDiscoveryCommand(l, venvtest.NewOSWithEmptyEnv().FS,
				&discovery.DiscoveryCommandOptions{
					WorkingDir: helpers.TmpDirWOSymlinks(t),
					Filters:    filters,
				})
			require.NoError(t, err)

			assert.Equal(t, tc.want, discoverysetup.FilteredPathsOnly(d, filters))
		})
	}
}
