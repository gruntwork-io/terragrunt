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
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_GraphParseStoresConfigConcurrentlyWithRacing reproduces, through
// the public Discover entry point, the data race that the per-Unit cfg/reading
// locks guard against.
//
// The graph phase parses a unit lazily, the first time it is reached, via
// ensureParsed -> parseComponent -> Unit.StoreConfig/SetReading. ensureParsed
// guards the parse with a check-then-act on Unit.Config(): if it is nil, parse
// and store. That check is not atomic across goroutines. With a `{./**}...`
// filter every unit is both a graph target (whose dependencies are walked in
// its own goroutine) and a dependency of other targets, so a shared unit is
// reached from two graph-phase goroutines at once. When both observe a nil
// config before either stores, both call StoreConfig/SetReading on the same
// Unit concurrently — a data race on cfg and reading.
//
// The window between the nil check and the store is the parse duration, so the
// shared unit is given a deliberately large config (a big remote_state map) to
// widen it enough that the race detector reliably observes the overlap. The
// fan-in of many leaves and the repeated Discover calls raise the odds that the
// two parsers overlap within a single test run.
//
// To confirm the locks are load-bearing, drop the lock/unlock calls from Unit's
// Config, StoreConfig, Reading, and SetReading and run this test with -race:
// the detector reports the race inside the graph phase
// (parseComponent -> Unit.StoreConfig). With the locks in place it passes.
func TestDiscovery_GraphParseStoresConfigConcurrentlyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// A large config widens the parse window so the concurrent re-parse is
	// observable. remote_state is partially decoded during discovery, so its
	// contents are actually walked rather than skipped.
	var sharedConfig strings.Builder

	sharedConfig.WriteString("remote_state {\n  backend = \"local\"\n")
	sharedConfig.WriteString("  generate = { path = \"backend.tf\", if_exists = \"overwrite\" }\n  config = {\n")

	for i := range 8000 {
		fmt.Fprintf(&sharedConfig, "    k%d = \"v%d\"\n", i, i)
	}

	sharedConfig.WriteString("  }\n}\n")

	vpcDir := filepath.Join(tmpDir, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(sharedConfig.String()), 0644))

	const leaves = 8

	for i := range leaves {
		leafDir := filepath.Join(tmpDir, fmt.Sprintf("app%d", i))
		require.NoError(t, os.MkdirAll(leafDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(leafDir, "terragrunt.hcl"), []byte(`
		dependency "vpc" {
			config_path = "../vpc"
		}
		`), 0644))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	for range 8 {
		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters)

		_, err := d.Discover(t.Context(), l, opts)
		require.NoError(t, err)
	}
}
