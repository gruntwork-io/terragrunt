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
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestDiscoveryPipelineMatchesPhasesForGitFiltersWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configure     func(d *discovery.Discovery) *discovery.Discovery
		name          string
		queries       []string
		omitGitFilter bool
	}{
		{name: "git filter"},
		{name: "git filter beside a path filter", queries: []string{"./untouched"}},
		{
			name:    "relationships",
			queries: []string{"./untouched"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships()
			},
		},
		{
			name:    "filter that reads the config",
			queries: []string{"reading=shared.hcl"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependents of a unit",
			queries: []string{"...db"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships().WithSuppressParseErrors()
			},
		},
		{
			name:          "dependencies and dependents of the changed units, without them",
			queries:       []string{"...^[HEAD~1...HEAD]..."},
			omitGitFilter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, runner := setupGitRepo(t)

			createUnit(t, repo, "db", "")
			createUnit(t, repo, "app", dependenciesOn("../db"))
			createUnit(t, repo, "web", dependenciesOn("../app"))
			createUnit(t, repo, "untouched", `locals { shared = read_terragrunt_config("../shared.hcl") }`)
			createUnit(t, repo, "removed", "")

			require.NoError(t, os.WriteFile(filepath.Join(repo, "shared.hcl"), []byte("locals {}\n"), 0o644))

			commitChanges(t, runner, "Initial commit")

			require.NoError(t, os.WriteFile(
				filepath.Join(repo, "app", "terragrunt.hcl"),
				[]byte(dependenciesOn("../db")+"locals {}\n"),
				0o644,
			))
			require.NoError(t, os.RemoveAll(filepath.Join(repo, "removed")))

			commitChanges(t, runner, "Change app and remove a unit")

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

			filters, err := filter.ParseFilterQueries(l, tt.queries)
			require.NoError(t, err)

			if !tt.omitGitFilter {
				filters = append(filter.Filters{filter.NewFilter(gitExpr, gitExpr.String())}, filters...)
			}

			discover := func(pipeline string) map[string][]string {
				opts := options.NewTerragruntOptions(vexec.NewOSExec())
				opts.WorkingDir = repo
				opts.RootWorkingDir = repo

				d := discovery.NewDiscovery(repo).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: repo}).
					WithWorktrees(w).
					WithFilters(filters)

				if tt.configure != nil {
					d = tt.configure(d)
				}

				v := venvtest.NewOSWithEmptyEnv()
				v.Env[discovery.EnvPipeline] = pipeline

				components, err := d.Discover(ctx, l, v, new(rewrite.Store), opts)
				require.NoError(t, err)

				return dependenciesByPath(components)
			}

			want := discover("false")
			require.NotEmpty(t, want)

			assert.Equal(t, want, discover("true"))
		})
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
