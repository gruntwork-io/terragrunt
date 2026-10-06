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

// TestDiscovery_DeletedDependency pins that a dependency on a unit or stack the Git diff deleted is reported as a
// [discovery.DeletedDependencyError], and that other missing dependencies are not.
func TestDiscovery_DeletedDependency(t *testing.T) {
	t.Parallel()

	const (
		dependsOnDep   = "dependency \"dep\" {\n  config_path = \"../dep\"\n}\n"
		dependsOnNever = "dependency \"never\" {\n  config_path = \"../never\"\n}\n"
		readsDep       = "locals {\n  dep = read_terragrunt_config(\"../dep/terragrunt.hcl\")\n}\n"
		stackFile      = "unit \"vpc\" {\n  source = \"../vpc\"\n  path   = \"vpc\"\n}\n"
	)

	tcs := []struct {
		name           string
		consumer       string
		filters        []string
		addDep         bool
		depIsStack     bool
		modifyConsumer bool
		wantDeleted    bool
	}{
		{
			name:        "untouched dependent in the working tree",
			consumer:    dependsOnDep,
			filters:     []string{"...[HEAD~1...HEAD]... | ./**"},
			wantDeleted: true,
		},
		{
			name:           "modified dependent with dependency expansion",
			consumer:       dependsOnDep,
			filters:        []string{"[HEAD~1...HEAD]..."},
			modifyConsumer: true,
			wantDeleted:    true,
		},
		{
			name:           "modified dependent",
			consumer:       dependsOnDep,
			filters:        []string{"[HEAD~1...HEAD]"},
			modifyConsumer: true,
			wantDeleted:    true,
		},
		{
			name:        "deleted dependency next to one missing at every reference",
			consumer:    dependsOnNever + dependsOnDep,
			filters:     []string{"...[HEAD~1...HEAD]... | ./**"},
			wantDeleted: true,
		},
		{
			name:        "deleted stack",
			consumer:    dependsOnDep,
			filters:     []string{"...[HEAD~1...HEAD]... | ./**"},
			depIsStack:  true,
			wantDeleted: true,
		},
		{
			name:           "dependency missing at every reference",
			consumer:       dependsOnNever,
			filters:        []string{"[HEAD~1...HEAD]"},
			modifyConsumer: true,
		},
		{
			name:     "dependency added in the diff",
			consumer: dependsOnDep,
			filters:  []string{"[HEAD~1...HEAD]", "./consumer"},
			addDep:   true,
		},
		{
			name:     "deleted unit read by read_terragrunt_config",
			consumer: readsDep,
			filters:  []string{"...[HEAD~1...HEAD]... | ./**"},
		},
		{
			name:     "no Git filter",
			consumer: dependsOnDep,
			filters:  []string{"./**"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupGitRepo(t)
			depDir := filepath.Join(tmpDir, "dep")

			// A second untouched unit keeps the relationship phase from stopping before it reaches the missing dep.
			createUnit(t, tmpDir, "other", `# other`)
			createUnit(t, tmpDir, "consumer", tc.consumer)

			switch {
			case tc.depIsStack:
				require.NoError(t, os.MkdirAll(depDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(depDir, config.DefaultStackFile), []byte(stackFile), 0o644))
			case !tc.addDep:
				createUnit(t, tmpDir, "dep", `# dep`)
			}

			commitChanges(t, runner, "Initial commit")

			if tc.modifyConsumer {
				createUnit(t, tmpDir, "consumer", tc.consumer+"# modified\n")
			}

			if tc.addDep {
				createUnit(t, tmpDir, "dep", `# dep`)
				commitChanges(t, runner, "Add dep")
				require.NoError(t, os.RemoveAll(depDir))
			} else {
				require.NoError(t, os.RemoveAll(depDir))
				commitChanges(t, runner, "Delete dep")
			}

			_, err := discoverForRunAll(t, tmpDir, tc.filters...)
			require.Error(t, err)

			deleted, ok := errors.AsType[discovery.DeletedDependencyError](err)
			if !tc.wantDeleted {
				assert.False(t, ok, "only a dependency on a unit deleted in the diff is a deleted dependency: %v", err)

				return
			}

			require.True(t, ok, "unexpected error %v", err)
			assert.Equal(t, "dep", deleted.Path)
			assert.Equal(t, "HEAD~1", deleted.Ref)
			assert.Contains(t, err.Error(), "a dependency points at dep, which exists at HEAD~1 but was deleted or moved")

			_, notFound := errors.AsType[config.TerragruntConfigNotFoundError](err)
			assert.True(t, notFound, "the original error must stay matchable: %v", err)
		})
	}
}

// discoverForRunAll runs discovery the way `run --all` configures it.
func discoverForRunAll(t *testing.T, workingDir string, filterQueries ...string) (component.Components, error) {
	t.Helper()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, filterQueries)
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
