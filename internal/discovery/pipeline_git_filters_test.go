package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestDiscoveryReadsTerragruntHCLBesideTheJSONConfigInGitFilterWorktreesWithRacing(t *testing.T) {
	t.Parallel()

	repo, runner := setupGitRepo(t)

	createUnit(t, repo, "app", "")
	createUnit(t, repo, "both", "")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "both", "terragrunt.hcl.json"), []byte("{"), 0o644))

	commitChanges(t, runner, "Initial commit")

	require.NoError(t, os.Remove(filepath.Join(repo, "both", "terragrunt.hcl.json")))

	commitChanges(t, runner, "Remove the JSON config")

	l := logger.CreateLogger()
	ctx := config.WithCaches(t.Context())

	gitExpr := filter.NewGitExpression("HEAD~1", "HEAD")

	w, err := worktrees.NewWorktrees(ctx, l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
		WorkingDir:     repo,
		GitExpressions: filter.GitExpressions{gitExpr},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.WithoutCancel(ctx), l, venvtest.NewOSWithEmptyEnv()))
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = repo
	opts.RootWorkingDir = repo

	components, err := discovery.NewDiscovery(repo).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: repo}).
		WithWorktrees(w).
		WithFilters(filter.Filters{filter.NewFilter(gitExpr, gitExpr.String())}).
		Discover(ctx, l, venvtest.NewOSWithEmptyEnv(), new(config.Store), opts)
	require.NoError(t, err)
	require.NotEmpty(t, components)

	for _, c := range components {
		assert.Equal(t, "both", filepath.Base(c.Path()))
		assert.Equal(t, config.DefaultTerragruntConfigPath, c.ConfigFile())
	}
}

func TestDiscoveryPipelineFindsWorktreeDependentsOfAnAbsolutePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workingDir string
	}{
		{name: "working directory at the Git root"},
		{name: "working directory below the Git root", workingDir: "live"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, runner := setupGitRepo(t)
			workingDir := filepath.Join(repo, tt.workingDir)

			createUnit(t, workingDir, "app", "")
			createUnit(t, workingDir, "shared", "")
			createUnit(t, workingDir, "pinned", dependenciesOn(
				filepath.Join(workingDir, "app"),
				filepath.Join(workingDir, "shared"),
			))

			commitChanges(t, runner, "Initial commit")

			require.NoError(t, os.WriteFile(filepath.Join(workingDir, "app", "terragrunt.hcl"), []byte("locals {}\n"), 0o644))

			commitChanges(t, runner, "Change app")

			l := logger.CreateLogger()
			ctx := config.WithCaches(t.Context())

			w, err := worktrees.NewWorktrees(ctx, l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
				WorkingDir:     workingDir,
				GitExpressions: filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")},
			})
			require.NoError(t, err)

			t.Cleanup(func() {
				require.NoError(t, w.Cleanup(context.WithoutCancel(ctx), l, venvtest.NewOSWithEmptyEnv()))
			})

			filters, err := filter.ParseFilterQueries(l, []string{"...[HEAD~1...HEAD]"})
			require.NoError(t, err)

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = workingDir
			opts.RootWorkingDir = workingDir

			components, err := discovery.NewDiscovery(workingDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
				WithWorktrees(w).
				WithFilters(filters).
				Discover(ctx, l, venvtest.NewOSWithEmptyEnv(), new(config.Store), opts)
			require.NoError(t, err)

			require.Len(t, w.WorktreePairs, 1)

			var head string
			for _, pair := range w.WorktreePairs {
				head = filepath.Join(pair.ToWorktree.Path, tt.workingDir)
			}

			app := filepath.Join(head, "app")

			assert.Equal(t, map[string][]string{
				app:                           {},
				filepath.Join(head, "pinned"): {filepath.Join(workingDir, "shared"), app},
			}, dependenciesByPath(components))
		})
	}
}

func TestDiscoveryPipelineParsesComponentsFoundInWorktreesWithRacing(t *testing.T) {
	t.Parallel()

	repo, runner := setupGitRepo(t)

	createUnit(t, repo, "kept", "")
	createUnit(t, repo, "removed", `locals { shared = read_terragrunt_config("../shared.hcl") }`+"\n")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "shared.hcl"), []byte("locals {}\n"), 0o644))

	commitChanges(t, runner, "Initial commit")

	require.NoError(t, os.RemoveAll(filepath.Join(repo, "removed")))

	commitChanges(t, runner, "Remove a unit")

	l := logger.CreateLogger()
	ctx := config.WithCaches(t.Context())

	gitExpr := filter.NewGitExpression("HEAD~1", "HEAD")

	w, err := worktrees.NewWorktrees(ctx, l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
		WorkingDir:     repo,
		GitExpressions: filter.GitExpressions{gitExpr},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.WithoutCancel(ctx), l, venvtest.NewOSWithEmptyEnv()))
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = repo
	opts.RootWorkingDir = repo

	components, err := discovery.NewDiscovery(repo).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: repo}).
		WithWorktrees(w).
		WithFilters(filter.Filters{filter.NewFilter(gitExpr, gitExpr.String())}).
		WithReadFiles().
		Discover(ctx, l, venvtest.NewOSWithEmptyEnv(), new(config.Store), opts)
	require.NoError(t, err)
	require.Len(t, components, 1)

	removed := components[0]

	assert.Equal(t, "removed", filepath.Base(removed.Path()))
	assert.Equal(t, []string{filepath.Join(filepath.Dir(removed.Path()), "shared.hcl")}, removed.Reading())
}

func TestDiscoveryPipelineReportsConfigConflictsInWorktreesWithRacing(t *testing.T) {
	t.Parallel()

	flows := []struct {
		configure func(d *discovery.Discovery) *discovery.Discovery
		name      string
	}{
		{
			name: "parse errors reported",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d
			},
		},
		{
			name: "parse errors suppressed",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
	}

	placements := []struct {
		commit func(t *testing.T, repo string, runner *git.GitRunner, addConflict func())
		name   string
	}{
		{
			name: "in the live checkout",
			commit: func(t *testing.T, _ string, runner *git.GitRunner, addConflict func()) {
				t.Helper()

				commitChanges(t, runner, "Initial commit")

				addConflict()

				commitChanges(t, runner, "Add a directory with two configs")
			},
		},
		{
			name: "only at the compared ref",
			commit: func(t *testing.T, repo string, runner *git.GitRunner, addConflict func()) {
				t.Helper()

				addConflict()

				commitChanges(t, runner, "Initial commit")

				require.NoError(t, os.RemoveAll(filepath.Join(repo, "both")))

				commitChanges(t, runner, "Remove the directory with two configs")
			},
		},
	}

	for _, placement := range placements {
		for _, flow := range flows {
			t.Run(placement.name+"/"+flow.name, func(t *testing.T) {
				t.Parallel()

				repo, runner := setupGitRepo(t)

				createUnit(t, repo, "kept", "")

				placement.commit(t, repo, runner, func() {
					both := createUnit(t, repo, "both", "")
					require.NoError(t, os.WriteFile(
						filepath.Join(both, config.DefaultStackFile),
						nil,
						0o644,
					))
				})

				l := logger.CreateLogger()
				ctx := config.WithCaches(t.Context())

				gitExpr := filter.NewGitExpression("HEAD~1", "HEAD")

				w, err := worktrees.NewWorktrees(ctx, l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
					WorkingDir:     repo,
					GitExpressions: filter.GitExpressions{gitExpr},
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					require.NoError(t, w.Cleanup(context.WithoutCancel(ctx), l, venvtest.NewOSWithEmptyEnv()))
				})

				opts := options.NewTerragruntOptions()
				opts.WorkingDir = repo
				opts.RootWorkingDir = repo

				d := flow.configure(discovery.NewDiscovery(repo).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: repo}).
					WithWorktrees(w).
					WithFilters(filter.Filters{filter.NewFilter(gitExpr, gitExpr.String())}))

				_, err = d.Discover(ctx, l, venvtest.NewOSWithEmptyEnv(), new(config.Store), opts)
				require.ErrorAs(t, err, new(discovery.CoexistenceError))
			})
		}
	}
}

// dependenciesByPath maps the path of each of components to the sorted paths of its dependencies.
func dependenciesByPath(components component.Components) map[string][]string {
	byPath := make(map[string][]string, len(components))

	for _, c := range components {
		paths := c.Dependencies().Paths()
		slices.Sort(paths)

		byPath[c.Path()] = paths
	}

	return byPath
}
