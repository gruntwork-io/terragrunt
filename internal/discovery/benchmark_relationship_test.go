package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// relationshipChainUnitCounts sizes the dependency chain fixtures. Every count
// is a multiple of relationshipChainLength.
var relationshipChainUnitCounts = []int{10, 100, 1_000, 10_000}

const relationshipChainLength = 10

// BenchmarkRelationshipDiscoveryChains benchmarks discovery with dependencies,
// as `find --dependencies` runs it, over a repository made of dependency chains.
//
// Discovery runs from live/, where each chain holds relationshipChainLength
// units and every unit but the first depends on the one before it. The first
// unit of each chain depends on a unit under modules/, outside the working
// directory, which relationship discovery has to parse and descend into.
func BenchmarkRelationshipDiscoveryChains(b *testing.B) {
	for _, n := range relationshipChainUnitCounts {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			tmpDir := helpers.TmpDirWOSymlinks(b)
			createRelationshipChainFixture(b, tmpDir, n/relationshipChainLength)

			workingDir := filepath.Join(tmpDir, "live")

			l := newDiscardLogger()
			opts := &options.TerragruntOptions{
				WorkingDir:        workingDir,
				RootWorkingDir:    workingDir,
				MaxFoldersToCheck: options.DefaultMaxFoldersToCheck,
			}
			v := venvtest.NewOSWithEmptyEnv()

			for b.Loop() {
				d := discovery.NewDiscovery(workingDir).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
					WithRequiresParse().
					WithRelationships()

				components, err := d.Discover(config.WithCaches(b.Context()), l, v, opts)
				require.NoError(b, err)
				require.Len(b, components, n)

				for _, c := range components {
					require.Len(b, c.Dependencies(), 1)
				}
			}
		})
	}
}

// createRelationshipChainFixture writes the given number of chains under
// tmpDir/live and one module unit per chain under tmpDir/modules.
func createRelationshipChainFixture(tb testing.TB, tmpDir string, chains int) {
	tb.Helper()

	files := make(map[string]string, chains*(relationshipChainLength+1))

	for c := range chains {
		module := fmt.Sprintf("module-%05d", c)
		files[filepath.Join("modules", module, "terragrunt.hcl")] = "# module\n"

		chainDir := filepath.Join("live", fmt.Sprintf("chain-%05d", c))
		dep := "../../../modules/" + module

		for u := range relationshipChainLength {
			unit := fmt.Sprintf("unit-%02d", u)
			files[filepath.Join(chainDir, unit, "terragrunt.hcl")] = fmt.Sprintf(
				"dependency \"prev\" {\n  config_path = %q\n}\n",
				dep,
			)
			dep = "../" + unit
		}
	}

	for name, contents := range files {
		path := filepath.Join(tmpDir, name)
		require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(tb, os.WriteFile(path, []byte(contents), 0644))
	}
}
