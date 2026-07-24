package config_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// BenchmarkFindInParentFolders measures resolving the root config for every
// unit in a run, the shape `run --all` produces. All units in a sub-tree share
// an ancestor chain, so each one re-probes the directories its siblings already
// probed. Depth is how many levels separate a unit from the root config.
func BenchmarkFindInParentFolders(b *testing.B) {
	for _, depth := range []int{1, 4, 8} {
		for _, units := range []int{10, 100} {
			configPaths := benchUnitTree(b, depth, units)

			b.Run("depth="+strconv.Itoa(depth)+"/units="+strconv.Itoa(units), func(b *testing.B) {
				l := logger.CreateLogger()
				baseCtx, pctx := newTestParsingContext(b, configPaths[0])
				params := []string{benchRootFileName}

				for b.Loop() {
					// A fresh cache per iteration: caches are per run, so a run
					// never starts warm.
					ctx := config.WithConfigValues(baseCtx)

					for _, configPath := range configPaths {
						pctx.TerragruntConfigPath = configPath

						if _, err := config.FindInParentFolders(ctx, pctx, l, params); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

const benchRootFileName = "root.hcl"

// benchUnitTree lays out units nested depth levels below a root config and
// returns their config paths. The intermediate directories are shared by every
// unit, which is what makes the walks redundant.
func benchUnitTree(b *testing.B, depth, units int) []string {
	b.Helper()

	const (
		dirPerm  = 0755
		filePerm = 0644
	)

	root := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(root, benchRootFileName), nil, filePerm))

	parent := root
	for i := range depth - 1 {
		parent = filepath.Join(parent, "level-"+strconv.Itoa(i))
	}

	configPaths := make([]string, units)

	for i := range units {
		unitDir := filepath.Join(parent, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, dirPerm))

		configPaths[i] = filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
		require.NoError(b, os.WriteFile(configPaths[i], nil, filePerm))
	}

	return configPaths
}
