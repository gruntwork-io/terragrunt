package discovery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithRelationships()

	components, err := d.Discover(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(config.Store), opts)
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

				opts := options.NewTerragruntOptions()
				opts.WorkingDir = tmpDir
				opts.RootWorkingDir = tmpDir

				d := discovery.NewDiscovery(tmpDir).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
					WithRelationships()

				components, err := d.Discover(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(config.Store), opts)
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

// TestRelationshipPhase_StopsWhenCanceledWithRacing pins that discovery of a
// unit with many dependencies stops once its context is canceled, wherever in
// parsing or relationship discovery the cancellation lands. It counts the stats
// of dependency directories in a full run, then cancels at points spread across
// that count, and expects no more than the dependency in progress to be stat'd
// after each.
func TestRelationshipPhase_StopsWhenCanceledWithRacing(t *testing.T) {
	t.Parallel()

	const cuts = 8

	full := &cancelOnShardFS{}
	require.NoError(t, discoverShards(t, t.Context(), full))

	for i := 1; i < cuts; i++ {
		cancelAt := full.stats * i / cuts

		t.Run(fmt.Sprintf("cancel at stat %d of %d", cancelAt, full.stats), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			fsys := &cancelOnShardFS{cancel: cancel, cancelAt: cancelAt}

			err := discoverShards(t, ctx, fsys)
			require.ErrorIs(t, err, context.Canceled)

			assert.LessOrEqual(t, len(fsys.statsAfterCancel), 1)
		})
	}
}

// discoverShards discovers the relationships of a unit that depends on 1000
// shard-* units, with every filesystem call going through fsys, and returns the
// error of discovery. Discovery runs on one worker, so one dependency at a time
// is in progress.
func discoverShards(t *testing.T, ctx context.Context, fsys *cancelOnShardFS) error {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	v := memRepoRootVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		filepath.Join(tmpDir, "app"): `
dependency "shard" {
  expansion {
    count = 1000
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true
}
`,
	})

	fsys.FS = v.FS

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithNumWorkers(1).
		WithRelationships()

	_, err := d.Discover(config.WithCaches(ctx), logger.CreateLogger(), v.WithFS(fsys), new(rewrite.Store), opts)

	return err
}

// cancelOnShardFS counts the stats of shard-* directories. With cancel set, it
// calls cancel on stat number cancelAt and records each shard directory stat'd
// after that.
type cancelOnShardFS struct {
	vfs.FS
	cancel           context.CancelFunc
	statsAfterCancel map[string]struct{}
	cancelAt         int
	stats            int
	mu               sync.Mutex
}

func (f *cancelOnShardFS) Stat(name string) (os.FileInfo, error) {
	if dir := filepath.Base(name); strings.HasPrefix(dir, "shard-") {
		f.mu.Lock()

		f.stats++

		if f.cancel != nil && f.stats > f.cancelAt {
			if f.statsAfterCancel == nil {
				f.statsAfterCancel = map[string]struct{}{}
			}

			f.statsAfterCancel[dir] = struct{}{}
		}

		if f.cancel != nil && f.stats == f.cancelAt {
			f.cancel()
		}

		f.mu.Unlock()
	}

	return f.FS.Stat(name)
}
