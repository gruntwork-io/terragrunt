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

// TestDiscovery_GraphConcurrentConfigAccessWithRacing reproduces, through the
// public Discover entry point, the data race the per-Unit cfg/reading locks
// guard against: the graph phase reaches a shared unit from several goroutines
// at once, so one goroutine's parse stores the config while others read it.
//
// The shared unit gets a large config and many dependents, repeated across
// several Discover calls, so the read/write overlap is reliably observable; a
// smaller config or fewer iterations make the race intermittent.
//
// To confirm the locks are load-bearing, drop the lock/unlock calls from Unit's
// Config, StoreConfig, Reading, and SetReading and run with -race.
func TestDiscovery_GraphConcurrentConfigAccessWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// remote_state is partially decoded during discovery, so a large block is
	// walked during parse rather than skipped, which lengthens the parse.
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

		_, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
		require.NoError(t, err)
	}
}
