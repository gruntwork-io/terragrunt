package discovery_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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

	v := memRepoRootVenv(t, tmpDir)

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

	components, err := d.Discover(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)
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

// TestRelationshipPhase_UnitDependencyBlockReplacesIncludedBlock pins that a
// unit depends only on the dependency blocks left after include merging, so a
// unit that overrides or disables an included block drops its path.
func TestRelationshipPhase_UnitDependencyBlockReplacesIncludedBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		unitBlock string
		want      []string
	}{
		{
			name: "path override",
			unitBlock: `
dependency "db" {
  config_path = "../replica"
}
`,
			want: []string{"replica"},
		},
		{
			name: "disabled",
			unitBlock: `
dependency "db" {
  config_path = "../db"
  enabled     = false
}
`,
			want: []string{},
		},
	}

	for _, tt := range tests {
		for _, strategy := range []string{"shallow", "deep"} {
			t.Run(tt.name+" "+strategy, func(t *testing.T) {
				t.Parallel()

				tmpDir := helpers.TmpDirWOSymlinks(t)
				appDir := filepath.Join(tmpDir, "app")

				v := memRepoRootVenv(t, tmpDir)

				require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(tmpDir, "root.hcl"), []byte(`
dependency "db" {
  config_path = "${get_terragrunt_dir()}/../db"
}
`), 0o644))

				writeUnits(t, v.FS, map[string]string{
					filepath.Join(tmpDir, "db"):      ``,
					filepath.Join(tmpDir, "replica"): ``,
					appDir: fmt.Sprintf(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = %q
}
%s`, strategy, tt.unitBlock),
				})

				opts := options.NewTerragruntOptions(vexec.NewOSExec())
				opts.WorkingDir = tmpDir
				opts.RootWorkingDir = tmpDir

				d := discovery.NewDiscovery(tmpDir).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
					WithRelationships()

				components, err := d.Discover(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)
				require.NoError(t, err)

				app := components.FilterByPath(appDir)
				require.Len(t, app, 1)

				want := make([]string, 0, len(tt.want))
				for _, dir := range tt.want {
					want = append(want, filepath.Join(tmpDir, dir))
				}

				assert.ElementsMatch(t, want, app[0].Dependencies().Paths())
			})
		}
	}
}
