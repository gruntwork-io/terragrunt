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

// TestDiscovery_GraphConcurrentConfigAccessWithRacing reproduces, through the
// public Discover entry point, the data race that the per-Unit cfg/reading
// locks guard against.
//
// The graph phase parses a unit lazily, via ensureParsed -> parseComponent ->
// Unit.StoreConfig/SetReading. With a `{./**}...` filter every unit is both a
// graph target (walked in its own goroutine) and a dependency of other targets,
// so a shared unit is reached by several graph-phase goroutines at once.
// GuardConfigParse serializes the parse so the config is stored a single time.
// The goroutines that lose that race still read the unit's config concurrently
// with the winner's store: the ensureParsed cache check and the downstream
// dependency extraction both call Unit.Config(). Without the lock on
// Config/StoreConfig (and Reading/SetReading), that read/write pair is a data
// race on cfg and reading.
//
// The shared unit is given a deliberately large config (a big remote_state map)
// to lengthen the parse so the reads and the store reliably overlap. The fan-in
// of many leaves and the repeated Discover calls raise the odds of overlap
// within a single test run.
//
// To confirm the locks are load-bearing, drop the lock/unlock calls from Unit's
// Config, StoreConfig, Reading, and SetReading and run this test with -race:
// the detector reports the race between Unit.StoreConfig and Unit.Config inside
// the graph phase. With the locks in place it passes.
func TestDiscovery_GraphConcurrentConfigAccessWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// A large config lengthens the parse so the concurrent reads and the store
	// overlap. remote_state is partially decoded during discovery, so its
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
