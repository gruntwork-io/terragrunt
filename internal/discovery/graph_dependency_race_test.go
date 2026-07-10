package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestGraphPhase_ConcurrentDependencyDiscoveryWithRacing pins that graph
// dependency discovery assigns a component's discovery context before the
// component becomes reachable by other goroutines. The graph phase fans out one
// goroutine per dependency, and dependencies that resolve outside the discovered
// set are created during traversal. When a shared dependency is reached along
// several paths at once, no goroutine should observe it, and recurse into it,
// until its working directory has been set.
//
// The graph fans out from a single target to many parents that all share one
// intermediate node, rooted outside the working directory so the whole closure
// is created during traversal rather than pre-discovered. This drives many
// concurrent reaches at the shared node.
func TestGraphPhase_ConcurrentDependencyDiscoveryWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	workingDir := filepath.Join(tmpDir, "root")
	extDir := filepath.Join(tmpDir, "ext")

	const fanOut = 24

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

	// The dependency closure (parents, mid, leaf) lives under ext/, outside the
	// working directory, so it is not pre-discovered by the filesystem walk and
	// must be created during graph traversal. Every parent shares "mid", which
	// itself depends on "leaf", so a goroutine that observes mid before its
	// context is assigned recurses into it.
	writeUnit(extDir, "leaf")
	writeUnit(extDir, "mid", "../leaf")

	parents := make([]string, 0, fanOut)
	for i := range fanOut {
		name := fmt.Sprintf("parent%02d", i)
		writeUnit(extDir, name, "../mid")
		parents = append(parents, "../../ext/"+name)
	}

	writeUnit(workingDir, "target", parents...)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     workingDir,
		RootWorkingDir: workingDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{"target..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithFilters(filters)

	components, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.NoError(t, err)

	paths := components.Paths()
	require.Contains(t, paths, filepath.Join(extDir, "mid"))
	require.Contains(t, paths, filepath.Join(extDir, "leaf"))
}

// TestGraphPhase_ConcurrentUpstreamDownstreamSharedDependencyWithRacing pins that
// a dependency reached by both the upstream (dependents) and downstream
// (dependencies) traversals never becomes visible without a discovery context.
//
// Two targets are processed concurrently: one expanded through its dependents
// (...A) and one through its dependencies (B...). The upstream walk assigns
// dependency components to the shared set while checking whether each candidate
// depends on the target, and the downstream walk reaches the same components from
// the other side. Both sides converge on "shared" at once, so if a component is
// published before its working directory is set, a downstream reach writes its
// context while an upstream reach touches it, which the race detector flags.
//
// "shared" lives under ext/, outside the working directory, so it is created
// during traversal rather than pre-discovered with a context by the filesystem
// walk.
func TestGraphPhase_ConcurrentUpstreamDownstreamSharedDependencyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	workingDir := filepath.Join(tmpDir, "root")
	extDir := filepath.Join(tmpDir, "ext")

	const (
		upFanOut   = 24
		downFanOut = 24
	)

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

	// "shared" lives outside the working directory so it is not pre-discovered.
	writeUnit(extDir, "shared")

	// Target A (dependents side): many units depend on A and also pull in "shared".
	// The upstream walk publishes "shared" while checking each of these dependents.
	writeUnit(workingDir, "A")

	for i := range upFanOut {
		writeUnit(workingDir, fmt.Sprintf("up%02d", i), "../A", "../../ext/shared")
	}

	// Target B (dependencies side): fans out to many parents that all reach
	// "shared" from the downstream direction at the same time.
	downParents := make([]string, 0, downFanOut)
	for j := range downFanOut {
		name := fmt.Sprintf("p%02d", j)
		writeUnit(workingDir, name, "../../ext/shared")
		downParents = append(downParents, "../"+name)
	}

	writeUnit(workingDir, "B", downParents...)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     workingDir,
		RootWorkingDir: workingDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{
		"...{" + filepath.Join(workingDir, "A") + "}",
		"{" + filepath.Join(workingDir, "B") + "}...",
	})
	require.NoError(t, err)

	d := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(workingDir).
		WithFilters(filters)

	components, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.NoError(t, err)

	require.Contains(t, components.Paths(), filepath.Join(extDir, "shared"))

	// Every discovered component must carry a working directory; a component
	// published before its context was assigned would surface here as empty.
	for _, c := range components {
		dctx := c.DiscoveryContext()
		require.NotNilf(t, dctx, "component %s has no discovery context", c.Path())
		require.NotEmptyf(
			t,
			dctx.WorkingDir,
			"component %s has an empty working directory",
			c.Path(),
		)
	}
}
