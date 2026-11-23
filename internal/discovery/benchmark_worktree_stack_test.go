package discovery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// worktreeStackSource is how the catalog stack of a worktree stack fixture sources its units.
type worktreeStackSource int

const (
	// worktreeStackSourceCAS rewrites each unit source to a cas:: reference, so generation
	// materializes the units from the CAS store.
	worktreeStackSourceCAS worktreeStackSource = iota

	// worktreeStackSourceCopy sources each unit by path, so generation copies the units.
	worktreeStackSourceCopy
)

// BenchmarkWorktreeChangedStack benchmarks discovery of a changed stack in a pair of Git worktrees.
// The live stack file changes between the two commits and its generated units do not, so every
// unit pair is compared and found the same.
func BenchmarkWorktreeChangedStack(b *testing.B) {
	sources := []struct {
		name   string
		source worktreeStackSource
	}{
		{name: "cas", source: worktreeStackSourceCAS},
		{name: "copy", source: worktreeStackSourceCopy},
	}

	for _, n := range []int{100, 1000} {
		for _, src := range sources {
			b.Run(fmt.Sprintf("units_%d/%s", n, src.name), func(b *testing.B) {
				benchmarkWorktreeChangedStack(b, n, src.source)
			})
		}
	}
}

// benchmarkWorktreeChangedStack generates a fixture of n units sourced by source in both
// worktrees, then times discovery alone.
func benchmarkWorktreeChangedStack(b *testing.B, n int, source worktreeStackSource) {
	b.Helper()

	ctx := config.WithCaches(b.Context())
	l := newDiscardLogger()

	home := b.TempDir()
	cacheDir := b.TempDir()
	v := venvtest.NewOSWithEmptyEnv().
		WithEnv(map[string]string{"HOME": home}).
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithUserCacheDir(func() (string, error) { return cacheDir, nil })

	root := helpers.TmpDirWOSymlinks(b)
	createWorktreeStackFixture(ctx, b, root, helpers.TmpDirWOSymlinks(b), n, source)

	const query = "[HEAD~1...HEAD]"

	w, err := worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
		WorkingDir:     root,
		GitExpressions: filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")},
	})
	require.NoError(b, err)

	b.Cleanup(func() {
		require.NoError(b, w.Cleanup(context.WithoutCancel(ctx), l, v))
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = root
	opts.RootWorkingDir = root

	filters, err := filter.ParseFilterQueries(l, []string{query})
	require.NoError(b, err)

	opts.Filters = filters

	require.NoError(b, generate.NewGenerator().GenerateStacks(ctx, l, v, new(config.Store), opts, w))

	toStackDir := filepath.Join(w.WorktreePairs[query].ToWorktree.Path, "live")
	requireGeneratedUnits(b, toStackDir, n)

	for b.Loop() {
		components, err := discovery.NewDiscovery(root).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: root, Cmd: "plan"}).
			WithWorktrees(w).
			WithFilters(filters).
			Discover(ctx, l, v, new(config.Store), opts)
		require.NoError(b, err)

		for _, c := range components {
			require.NotContains(b, c.Path(), "unit_", "unchanged unit %s discovered as changed", c.Path())
		}
	}
}

// createWorktreeStackFixture writes a catalog stack of n units into catalogDir, commits a live
// stack sourcing it into a repository at root, then commits a change to the live stack file alone.
func createWorktreeStackFixture(
	ctx context.Context,
	b *testing.B,
	root, catalogDir string,
	n int,
	source worktreeStackSource,
) {
	b.Helper()

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(b, err)

	runner = runner.WithWorkDir(root)

	require.NoError(b, runner.Init(ctx))
	require.NoError(b, runner.ConfigSet(ctx, "user.email", "test@example.com"))
	require.NoError(b, runner.ConfigSet(ctx, "user.name", "Test User"))
	require.NoError(b, runner.ConfigSet(ctx, "commit.gpgsign", "false"))

	unitDir := filepath.Join(catalogDir, "units", "unit")
	unitFiles := map[string]string{
		"terragrunt.hcl": "inputs = {\n  index = values.index\n}\n",
		"main.tf":        "resource \"terraform_data\" \"this\" {\n  input = var.index\n}\n",
		"variables.tf":   "variable \"index\" {\n  type = number\n}\n",
		"outputs.tf":     "output \"index\" {\n  value = terraform_data.this.output\n}\n",
	}

	for name, content := range unitFiles {
		writeBenchmarkFile(b, filepath.Join(unitDir, name), content)
	}

	var stack strings.Builder

	for i := range n {
		switch source {
		case worktreeStackSourceCAS:
			fmt.Fprintf(&stack, "unit \"unit_%05d\" {\n  source = \"../..//units/unit\"\n  update_source_with_cas = true\n", i)
		case worktreeStackSourceCopy:
			fmt.Fprintf(&stack, "unit \"unit_%05d\" {\n  source = %q\n", i, filepath.ToSlash(unitDir))
		}

		fmt.Fprintf(&stack, "  path = \"unit_%05d\"\n  values = {\n    index = %d\n  }\n}\n\n", i, i)
	}

	writeBenchmarkFile(b, filepath.Join(catalogDir, "stacks", "stack", config.DefaultStackFile), stack.String())

	liveStackFile := filepath.Join(root, "live", config.DefaultStackFile)
	liveStack := fmt.Sprintf(
		"stack \"stack\" {\n  source = %q\n  path   = \"stack\"\n}\n",
		filepath.ToSlash(catalogDir)+"//stacks/stack",
	)

	writeBenchmarkFile(b, liveStackFile, liveStack)
	require.NoError(b, runner.Add(ctx, "."))
	require.NoError(b, runner.Commit(ctx, "Create live stack"))

	writeBenchmarkFile(b, liveStackFile, "# Changed\n"+liveStack)
	require.NoError(b, runner.Add(ctx, "."))
	require.NoError(b, runner.Commit(ctx, "Change live stack"))
}

// requireGeneratedUnits fails b unless generation wrote n units below the stack in stackDir.
func requireGeneratedUnits(b *testing.B, stackDir string, n int) {
	b.Helper()

	units, err := filepath.Glob(filepath.Join(stackDir, ".terragrunt-stack", "stack", ".terragrunt-stack", "unit_*"))
	require.NoError(b, err)
	require.Len(b, units, n)
}

// writeBenchmarkFile writes content to path, creating its parent directories.
func writeBenchmarkFile(b *testing.B, path, content string) {
	b.Helper()

	require.NoError(b, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(b, os.WriteFile(path, []byte(content), 0o644))
}
