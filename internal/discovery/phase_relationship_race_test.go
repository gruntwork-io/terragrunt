package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestRelationshipPhase_ConcurrentSharedDependencyWithRacing pins that a
// transient dependency's discovery context is written only by the goroutine that
// creates it. When many components depend on one unit outside the discovered set,
// several goroutines reach it at once; the context accessors are unlocked, so any
// extra writer races. The shared unit lives under ext/ so it is created during
// traversal rather than pre-discovered by the filesystem walk.
func TestRelationshipPhase_ConcurrentSharedDependencyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	workingDir := filepath.Join(tmpDir, "root")
	extDir := filepath.Join(tmpDir, "ext")

	const fanOut = 8

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

	writeUnit(extDir, "shared")

	for i := range fanOut {
		writeUnit(workingDir, fmt.Sprintf("unit%02d", i), "../../ext/shared")
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     workingDir,
		RootWorkingDir: workingDir,
	}

	d := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithRelationships()

	components, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.NoError(t, err)

	sharedPath := filepath.Join(extDir, "shared")

	for _, c := range components {
		deps := c.Dependencies().Paths()
		require.Containsf(t, deps, sharedPath, "unit %s missing shared dependency", c.Path())
	}
}
