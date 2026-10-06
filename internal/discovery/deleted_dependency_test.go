package discovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_DeletedDependency pins that a dependency on a unit the Git diff deleted is reported as a
// [discovery.DeletedDependencyError], and that other missing dependencies are not.
func TestDiscovery_DeletedDependency(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name           string
		filter         string
		dependencyPath string
		modifyConsumer bool
		wantDeleted    bool
	}{
		{
			name:           "untouched dependent in the working tree",
			filter:         "...[HEAD~1...HEAD]... | ./**",
			dependencyPath: "../dep",
			wantDeleted:    true,
		},
		{
			name:           "modified dependent with dependency expansion",
			filter:         "[HEAD~1...HEAD]...",
			dependencyPath: "../dep",
			modifyConsumer: true,
			wantDeleted:    true,
		},
		{
			name:           "modified dependent",
			filter:         "[HEAD~1...HEAD]",
			dependencyPath: "../dep",
			modifyConsumer: true,
			wantDeleted:    true,
		},
		{
			name:           "dependency missing at every reference",
			filter:         "[HEAD~1...HEAD]",
			dependencyPath: "../never",
			modifyConsumer: true,
		},
		{
			name:           "no Git filter",
			filter:         "./**",
			dependencyPath: "../dep",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupGitRepo(t)

			consumer := `dependency "dep" {
  config_path = "` + tc.dependencyPath + `"
}
`

			createUnit(t, tmpDir, "dep", `# dep`)
			createUnit(t, tmpDir, "other", `# other`)
			createUnit(t, tmpDir, "consumer", consumer)
			commitChanges(t, runner, "Initial commit")

			require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "dep")))

			if tc.modifyConsumer {
				createUnit(t, tmpDir, "consumer", consumer+"# modified\n")
			}

			commitChanges(t, runner, "Delete dep")

			_, err := discoverForRunAll(t, tmpDir, tc.filter)
			require.Error(t, err)

			_, notFound := errors.AsType[config.TerragruntConfigNotFoundError](err)
			assert.True(t, notFound, "unexpected error %v", err)

			deleted, ok := errors.AsType[discovery.DeletedDependencyError](err)
			if !tc.wantDeleted {
				assert.False(t, ok, "only a unit deleted in the diff is a deleted dependency: %v", err)

				return
			}

			require.True(t, ok, "unexpected error %v", err)
			assert.Equal(t, "dep", deleted.Path)
			assert.Equal(t, "HEAD~1", deleted.Ref)
		})
	}
}

// discoverForRunAll runs discovery the way `run --all` configures it.
func discoverForRunAll(t *testing.T, workingDir, filterQuery string) (component.Components, error) {
	t.Helper()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{filterQuery})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir
	opts.Filters = filters
	opts.Experiments = experiment.NewExperiments()

	d := discovery.NewDiscovery(workingDir).
		WithRelationships().
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir, Cmd: "plan"}).
		WithFilters(filters)

	if gitFilters := filters.UniqueGitFilters(); len(gitFilters) > 0 {
		w, wtErr := worktrees.NewWorktrees(t.Context(), l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
			WorkingDir:     workingDir,
			GitExpressions: gitFilters,
		})
		require.NoError(t, wtErr)

		t.Cleanup(func() {
			require.NoError(t, w.Cleanup(context.WithoutCancel(t.Context()), l, venvtest.NewOSWithEmptyEnv()))
		})

		d = d.WithWorktrees(w)
	}

	return d.Discover(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts)
}
