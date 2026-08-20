package discovery_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_GraphConcurrentConfigAccessWithRacing reproduces, through the
// public Discover entry point, the data race the per-Unit cfg/reading locks
// guard against: the graph phase reaches a shared unit from several goroutines
// at once, so one goroutine's parse stores the config while others read it.
//
// The shared unit gets a large config and many dependents, so the read/write
// overlap is reliably observable. The dependent count is the load-bearing knob:
// at six dependents the overlap stops being observable at all, so the fixture
// carries double the smallest count that detects the race on every run. The
// whole repo lives on an in-memory filesystem, so only parsing costs wall time
// and a single Discover call suffices.
//
// To confirm the locks are load-bearing, drop the lock/unlock calls from Unit's
// Config, StoreConfig, Reading, and SetReading and run with -race.
func TestDiscovery_GraphConcurrentConfigAccessWithRacing(t *testing.T) {
	t.Parallel()

	repoRoot := string(filepath.Separator) + "repo"

	v := memGitTopLevelVenv(t, repoRoot)

	// remote_state is partially decoded during discovery, so a large block is
	// walked during parse rather than skipped, which lengthens the parse.
	var sharedConfig strings.Builder

	sharedConfig.WriteString("remote_state {\n  backend = \"local\"\n")
	sharedConfig.WriteString(
		"  generate = { path = \"backend.tf\", if_exists = \"overwrite\" }\n  config = {\n",
	)

	for i := range 2000 {
		fmt.Fprintf(&sharedConfig, "    k%d = \"v%d\"\n", i, i)
	}

	sharedConfig.WriteString("  }\n}\n")

	units := map[string]string{
		filepath.Join(repoRoot, "vpc"): sharedConfig.String(),
	}

	const leaves = 16

	for i := range leaves {
		units[filepath.Join(repoRoot, fmt.Sprintf("app%d", i))] = `
		dependency "vpc" {
			config_path = "../vpc"
		}
		`
	}

	writeUnits(t, v.FS, units)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     repoRoot,
		RootWorkingDir: repoRoot,
	}

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(repoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: repoRoot}).
		WithFilters(filters)

	_, err = d.Discover(t.Context(), l, v, opts)
	require.NoError(t, err)
}
