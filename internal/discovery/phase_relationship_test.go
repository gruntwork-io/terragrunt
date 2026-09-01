package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationshipPhase_SortsDependencies pins that a unit's dependencies come
// back sorted by path. The unit below declares them in reverse alphabetical
// order, split across a dependency block and a dependencies block, so config
// order and sorted order are exact opposites.
func TestRelationshipPhase_SortsDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	appDir := filepath.Join(tmpDir, "app")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		filepath.Join(tmpDir, "a-unit"): ``,
		filepath.Join(tmpDir, "b-unit"): ``,
		filepath.Join(tmpDir, "c-unit"): ``,
		filepath.Join(tmpDir, "d-unit"): ``,
		appDir: `
dependency "d" {
  config_path = "../d-unit"
}

dependency "c" {
  config_path = "../c-unit"
}

dependencies {
  paths = ["../b-unit", "../a-unit"]
}
`,
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithRelationships()

	components, err := d.Discover(t.Context(), logger.CreateLogger(), v, opts)
	require.NoError(t, err)

	app := components.FilterByPath(appDir)
	require.Len(t, app, 1)

	want := []string{
		filepath.Join(tmpDir, "a-unit"),
		filepath.Join(tmpDir, "b-unit"),
		filepath.Join(tmpDir, "c-unit"),
		filepath.Join(tmpDir, "d-unit"),
	}
	assert.Equal(t, want, app[0].Dependencies().Paths())
}
