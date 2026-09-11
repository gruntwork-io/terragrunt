package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestGraphPhase_ConcurrentDependentsSharedParseWithRacing pins that many
// dependents of one target are discovered while sharing a single parse of each
// unit they reach.
//
// The upstream dependent walk mints a throwaway component for every config file
// it passes and canonicalises it against the shared component set, so concurrent
// candidates converge on one component per directory and race to parse it. Every
// dependent the walk finds restarts it over the same directories, so those
// components are reached again from the recursions while the first pass is still
// running.
//
// The dependents live above the working directory, where the filesystem walk
// never reaches them, so the upstream walk is what discovers all of them. They
// share a dependency with each other as well as the target, so the convergence
// is not only on the component the query names.
func TestGraphPhase_ConcurrentDependentsSharedParseWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	gitRoot := tmpDir
	workingDir := filepath.Join(tmpDir, "root")

	const fanIn = 24

	writeUnit := func(baseDir, name string, deps ...string) {
		dir := filepath.Join(baseDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))

		var hcl strings.Builder

		hcl.WriteString("# " + name + "\n")

		for i, dep := range deps {
			fmt.Fprintf(&hcl, "dependency \"d%d\" {\n  config_path = %q\n}\n", i, dep)
		}

		require.NoError(
			t,
			os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(hcl.String()), 0644),
		)
	}

	writeUnit(workingDir, "vpc")
	writeUnit(gitRoot, "shared", "../root/vpc")

	expected := make([]string, 0, fanIn+2)
	expected = append(
		expected,
		filepath.Join(workingDir, "vpc"),
		filepath.Join(gitRoot, "shared"),
	)

	for i := range fanIn {
		name := fmt.Sprintf("dep%02d", i)

		writeUnit(gitRoot, name, "../root/vpc", "../shared")

		expected = append(expected, filepath.Join(gitRoot, name))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     workingDir,
		RootWorkingDir: workingDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{
		"...{" + filepath.Join(workingDir, "vpc") + "}",
	})
	require.NoError(t, err)

	components, err := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(gitRoot).
		WithFilters(filters).
		Discover(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts)
	require.NoError(t, err)

	require.ElementsMatch(t, expected, components.Filter(component.UnitKind).Paths())
}

// TestGraphPhase_UnitSharingDirectoryWithStackWithRacing pins that the selection
// for a dependents query does not depend on how the walk's candidates interleave.
//
// Coexistence validation only covers what the filesystem walk reached, so a
// directory above the working directory can hold both a unit config and a stack
// config. Each mints its own candidate reporting that directory as its path, and
// the walk fans its candidates out concurrently, so the two race to be checked
// against the target. The walk records the paths it has already checked, and a
// stack claiming the path first would drop the unit from the selection along
// with every unit reachable only through it.
//
// The repetitions are what make the losing interleaving show up: a single run
// picks whichever candidate the scheduler happened to start first.
func TestGraphPhase_UnitSharingDirectoryWithStackWithRacing(t *testing.T) {
	t.Parallel()

	const (
		fixtureName = "unit sharing a directory with a stack above the working directory"
		attempts    = 40
	)

	corpus := dependentsCorpus()

	idx := slices.IndexFunc(corpus, func(tc dependentsCase) bool {
		return tc.name == fixtureName
	})
	require.NotEqual(t, -1, idx, "corpus has no entry named %q", fixtureName)

	tc := corpus[idx]

	for attempt := range attempts {
		require.Equal(t, tc.expected, tc.run(t), "attempt %d", attempt)
	}
}
