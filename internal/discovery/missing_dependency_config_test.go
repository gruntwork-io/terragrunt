package discovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
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

// TestDiscovery_MissingDependencyConfig pins that a dependency without a config fails naming the unit declaring it.
func TestDiscovery_MissingDependencyConfig(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		files         map[string]string
		name          string
		wantUnit      string
		wantDep       string
		queries       []string
		relationships bool
	}{
		{
			name: "relationship phase",
			files: map[string]string{
				"consumer/terragrunt.hcl": unitHCL("../dep"),
				"other/terragrunt.hcl":    "",
			},
			relationships: true,
			wantUnit:      repoPath("consumer"),
			wantDep:       repoPath("dep"),
		},
		{
			name: "relationship phase on a directory without a config",
			files: map[string]string{
				"consumer/terragrunt.hcl": unitHCL("../dep"),
				"other/terragrunt.hcl":    "",
				"dep/main.tf":             "",
			},
			relationships: true,
			wantUnit:      repoPath("consumer"),
			wantDep:       repoPath("dep"),
		},
		{
			name: "relationship phase names the nearest unit",
			files: map[string]string{
				"app/terragrunt.hcl":   unitHCL("../mid"),
				"mid/terragrunt.hcl":   unitHCL("../dep"),
				"other/terragrunt.hcl": "",
			},
			relationships: true,
			wantUnit:      repoPath("mid"),
			wantDep:       repoPath("dep"),
		},
		{
			name: "graph phase",
			files: map[string]string{
				"consumer/terragrunt.hcl": unitHCL("../dep"),
			},
			queries:  []string{"{" + repoPath("consumer") + "}..."},
			wantUnit: repoPath("consumer"),
			wantDep:  repoPath("dep"),
		},
		{
			name: "graph phase names the nearest unit",
			files: map[string]string{
				"app/terragrunt.hcl": unitHCL("../mid"),
				"mid/terragrunt.hcl": unitHCL("../dep"),
			},
			queries:  []string{"{" + repoPath("app") + "}..."},
			wantUnit: repoPath("mid"),
			wantDep:  repoPath("dep"),
		},
		{
			name: "dependencies block",
			files: map[string]string{
				"consumer/terragrunt.hcl": "dependencies {\n  paths = [\"../dep\"]\n}\n",
				"other/terragrunt.hcl":    "",
				"dep/main.tf":             "",
			},
			relationships: true,
			wantUnit:      repoPath("consumer"),
			wantDep:       repoPath("dep"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := discoverInMemory(t, tc.files, tc.queries, tc.relationships)
			require.Error(t, err)

			missing, ok := errors.AsType[config.MissingDependencyConfigError](err)
			require.True(t, ok, "unexpected error %v", err)
			assert.Equal(t, tc.wantUnit, missing.UnitPath)
			assert.Equal(t, tc.wantDep, missing.DependencyPath)
			assert.Contains(t, err.Error(), "where no Terragrunt configuration was found")

			notFound, ok := errors.AsType[config.TerragruntConfigNotFoundError](err)
			require.True(t, ok, "the original error must stay in the chain")
			assert.Equal(t, filepath.Join(tc.wantDep, config.DefaultTerragruntConfigPath), notFound.Path)
		})
	}
}

// TestDiscovery_DependencyParseErrorsKeepTheirError pins that other dependency parse failures are not relabelled.
func TestDiscovery_DependencyParseErrorsKeepTheirError(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		files         map[string]string
		name          string
		queries       []string
		relationships bool
	}{
		{
			name: "invalid dependency config",
			files: map[string]string{
				"consumer/terragrunt.hcl": unitHCL("../dep"),
				"dep/terragrunt.hcl":      "locals {\n",
			},
			queries: []string{"{" + repoPath("consumer") + "}..."},
		},
		{
			name: "dependency includes a missing file",
			files: map[string]string{
				"consumer/terragrunt.hcl": unitHCL("../dep"),
				"dep/terragrunt.hcl":      "include \"shared\" {\n  path = \"shared.hcl\"\n}\n",
			},
			queries: []string{"{" + repoPath("consumer") + "}..."},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := discoverInMemory(t, tc.files, tc.queries, tc.relationships)
			require.Error(t, err)

			_, ok := errors.AsType[config.MissingDependencyConfigError](err)
			assert.False(t, ok, "unexpected missing dependency error %v", err)
		})
	}
}

// TestDiscovery_ExistingDependencyConfig is the control: the same trees with the dependency present discover cleanly.
func TestDiscovery_ExistingDependencyConfig(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"consumer/terragrunt.hcl": unitHCL("../dep"),
		"other/terragrunt.hcl":    "",
		"dep/terragrunt.hcl":      "",
	}

	require.NoError(t, discoverInMemory(t, files, nil, true))
	require.NoError(t, discoverInMemory(t, files, []string{"{" + repoPath("consumer") + "}..."}, false))
}

// TestDiscovery_GitFilterDeletedDependency reproduces a Git filter diff deleting a unit a live unit still references.
func TestDiscovery_GitFilterDeletedDependency(t *testing.T) {
	t.Parallel()

	const consumerHCL = `dependency "dep" {
  config_path = "../dep"
  mock_outputs = { name = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}
`

	tcs := []struct {
		modify    map[string]string
		name      string
		query     string
		wantErr   bool
		withThird bool
	}{
		{
			name:    "live unit pulled in by a path filter",
			query:   "...[HEAD~1...HEAD]... | ./**",
			wantErr: true,
		},
		{
			name:      "live unit reached as a dependency in the to worktree",
			query:     "[HEAD~1...HEAD]...",
			modify:    map[string]string{"third": "dependency \"consumer\" {\n  config_path = \"../consumer\"\n}\n# modified\n"},
			withThird: true,
			wantErr:   true,
		},
		{
			name:  "Git filter alone discovers the dependent without failing",
			query: "...[HEAD~1...HEAD]...",
		},
		{
			name:   "reference removed with the unit",
			query:  "...[HEAD~1...HEAD]... | ./**",
			modify: map[string]string{"consumer": "# dependency removed\n"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupGitRepo(t)

			createUnit(t, tmpDir, "dep", "# dep\n")
			createUnit(t, tmpDir, "consumer", consumerHCL)

			if tc.withThird {
				createUnit(t, tmpDir, "third", "dependency \"consumer\" {\n  config_path = \"../consumer\"\n}\n")
			}

			commitChanges(t, runner, "Initial commit")

			require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "dep")))

			for unit, content := range tc.modify {
				createUnit(t, tmpDir, unit, content)
			}

			commitChanges(t, runner, "Delete dep")

			components, err := discoverWithGitFilter(t, tmpDir, tc.query)
			if !tc.wantErr {
				require.NoError(t, err)

				names := make([]string, 0, len(components))
				for _, c := range components {
					names = append(names, filepath.Base(c.Path()))
				}

				assert.Contains(t, names, "consumer")
				assert.Contains(t, names, "dep")

				return
			}

			require.Error(t, err)

			missing, ok := errors.AsType[config.MissingDependencyConfigError](err)
			require.True(t, ok, "unexpected error %v", err)
			assert.Equal(t, filepath.Join(tmpDir, "consumer"), missing.UnitPath)
			assert.Equal(t, filepath.Join(tmpDir, "dep"), missing.DependencyPath)
			assert.NotContains(t, err.Error(), "terragrunt-worktree", "worktree path leaked")
		})
	}
}

// discoverInMemory runs discovery over files rooted at the corpus repo on an in-memory filesystem.
func discoverInMemory(
	t *testing.T,
	files map[string]string,
	queries []string,
	relationships bool,
) error {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, corpusRepoRoot, files))
	l := logger.CreateLogger()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = corpusRepoRoot
	opts.RootWorkingDir = corpusRepoRoot

	d := discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot})

	if relationships {
		d = d.WithRelationships()
	}

	if len(queries) > 0 {
		filters, err := filter.ParseFilterQueries(l, queries)
		require.NoError(t, err)

		d = d.WithFilters(filters)
	}

	_, err := d.Discover(t.Context(), l, v, opts)

	return err
}

// discoverWithGitFilter mirrors run --all discovery for a Git filter query in tmpDir.
func discoverWithGitFilter(t *testing.T, tmpDir, query string) (component.Components, error) {
	t.Helper()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{query})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(t.Context(), l, venvtest.NewOSWithEmptyEnv(), worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.WithoutCancel(t.Context()), l, venvtest.NewOSWithEmptyEnv()))
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	return discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir, Cmd: "plan"}).
		WithRelationships().
		WithWorktrees(w).
		WithFilters(filters).
		Discover(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts)
}
