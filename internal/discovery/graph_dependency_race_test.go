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

		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(hcl.String()), 0644))
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
