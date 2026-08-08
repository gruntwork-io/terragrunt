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
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_GraphConcurrentConfigAccessWithRacing reproduces, through the
// public Discover entry point, the data race the per-Unit cfg/reading locks
// guard against: the graph phase reaches a shared unit from several goroutines
// at once, so one goroutine's parse stores the config while others read it.
//
// The shared unit gets a large config and many dependents, repeated across
// several Discover calls, so the read/write overlap is reliably observable;
// the config size and iteration count carry a margin over the smallest values
// that still detect the race on every run. All fixture files live on an
// in-memory filesystem, so only parsing costs wall time.
//
// To confirm the locks are load-bearing, drop the lock/unlock calls from Unit's
// Config, StoreConfig, Reading, and SetReading and run with -race.
func TestDiscovery_GraphConcurrentConfigAccessWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	v := memGitTopLevelVenv(t, tmpDir)

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
		filepath.Join(tmpDir, "vpc"): sharedConfig.String(),
	}

	const leaves = 8

	for i := range leaves {
		units[filepath.Join(tmpDir, fmt.Sprintf("app%d", i))] = `
		dependency "vpc" {
			config_path = "../vpc"
		}
		`
	}

	writeUnits(t, v.FS, units)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	for range 4 {
		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters)

		_, err := d.Discover(t.Context(), l, v, opts)
		require.NoError(t, err)
	}
}
