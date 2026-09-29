package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// liveLayoutEnvDirCount is the fixed number of account/region/env leaf
// directories a live layout fixture spreads its units across: 2 accounts, 4
// regions per account, 5 envs per region.
const liveLayoutEnvDirCount = 2 * 4 * 5

// liveLayoutQuery is one filter shape the live layout benchmark runs.
type liveLayoutQuery struct {
	queries       func(root string) []string
	name          string
	relationships bool
}

// BenchmarkLiveLayoutDiscovery benchmarks discovery over a fixture laid out
// like gruntwork-io/terragrunt-infrastructure-live-example: a root.hcl whose
// locals read account.hcl, region.hcl, and env.hcl through
// find_in_parent_folders, included by every unit.
func BenchmarkLiveLayoutDiscovery(b *testing.B) {
	shapes := []liveLayoutQuery{
		{
			name:          "all",
			queries:       func(string) []string { return nil },
			relationships: true,
		},
		{
			name:    "path",
			queries: func(string) []string { return []string{"./acct0/region0/**"} },
		},
		{
			name:    "path_literal",
			queries: func(string) []string { return []string{"./acct0/region0/env0/unit00001"} },
		},
		{
			name:    "path_wildcard",
			queries: func(string) []string { return []string{"./acct0/*/env0/*"} },
		},
		{
			name: "dependents",
			queries: func(root string) []string {
				return []string{"...{" + filepath.Join(root, "acct0", "region0", "env0", "unit00001") + "}"}
			},
		},
	}

	for _, n := range []int{1000, 5000, 10000} {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			root := b.TempDir()
			createLiveLayoutFixture(b, root, n)

			for _, shape := range shapes {
				b.Run(shape.name, func(b *testing.B) {
					benchmarkLiveLayout(b, root, shape)
				})
			}
		})
	}
}

// benchmarkLiveLayout runs discovery over the fixture at root with the filter
// shape. Parse errors fail the benchmark, so a fixture that stops parsing
// cannot pass as a fast one.
func benchmarkLiveLayout(b *testing.B, root string, shape liveLayoutQuery) {
	b.Helper()

	l := newDiscardLogger()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = root
	opts.RootWorkingDir = root

	filters, err := filter.ParseFilterQueries(l, shape.queries(root))
	require.NoError(b, err)

	v := venvtest.NewOSWithEmptyEnv()

	for b.Loop() {
		d := discovery.NewDiscovery(root).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: root}).
			WithGitRoot(root)

		if len(filters) > 0 {
			d = d.WithFilters(filters)
		}

		if shape.relationships {
			d = d.WithRelationships()
		}

		_, err := d.Discover(config.WithConfigValues(b.Context()), l, v, opts)
		require.NoError(b, err)
	}
}

// createLiveLayoutFixture writes a live layout fixture under root: a shared
// root.hcl, 2 accounts x 4 regions x 5 envs of locals-only config, and units
// units spread evenly across the resulting 40 env directories.
//
// It adds to the live example's layout: root.hcl also decodes a common.yaml
// read through file(), units sit directly in their env directory, and every
// third unit within an env directory depends on the unit created immediately
// before it.
func createLiveLayoutFixture(tb testing.TB, root string, units int) {
	tb.Helper()

	writeLiveLayoutRootFiles(tb, root)

	const (
		accounts = 2
		regions  = 4
		envs     = 5
	)

	envDirs := make([]string, 0, liveLayoutEnvDirCount)

	for a := range accounts {
		acctDir := filepath.Join(root, fmt.Sprintf("acct%d", a))
		writeLiveLayoutLocalsFile(tb, acctDir, "account.hcl")

		for r := range regions {
			regionDir := filepath.Join(acctDir, fmt.Sprintf("region%d", r))
			writeLiveLayoutLocalsFile(tb, regionDir, "region.hcl")

			for e := range envs {
				envDir := filepath.Join(regionDir, fmt.Sprintf("env%d", e))
				writeLiveLayoutLocalsFile(tb, envDir, "env.hcl")

				envDirs = append(envDirs, envDir)
			}
		}
	}

	writeLiveLayoutUnits(tb, envDirs, units)
}

// writeLiveLayoutRootFiles writes the shared root.hcl and common.yaml that
// every unit in a live layout fixture reads through find_in_parent_folders.
func writeLiveLayoutRootFiles(tb testing.TB, root string) {
	tb.Helper()

	rootHCL := `locals {
  account = read_terragrunt_config(find_in_parent_folders("account.hcl"))
  region  = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  env     = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common  = yamldecode(file(find_in_parent_folders("common.yaml")))
}
`
	require.NoError(tb, os.WriteFile(filepath.Join(root, "root.hcl"), []byte(rootHCL), 0o644))

	var yaml strings.Builder

	for i := range 80 {
		fmt.Fprintf(&yaml, "key%02d: \"common-config-value-%02d-%s\"\n", i, i, strings.Repeat("x", 20))
	}

	require.NoError(tb, os.WriteFile(filepath.Join(root, "common.yaml"), []byte(yaml.String()), 0o644))
}

// writeLiveLayoutLocalsFile creates dir and writes a locals-only HCL file
// named filename into it, with 5 string values. Each value after the first
// references the one before it, so evaluating the file resolves a chain of
// locals.
func writeLiveLayoutLocalsFile(tb testing.TB, dir, filename string) {
	tb.Helper()

	require.NoError(tb, os.MkdirAll(dir, 0o755))

	content := `locals {
  key1 = "value1"
  key2 = "${local.key1}-value2"
  key3 = "${local.key2}-value3"
  key4 = "${local.key3}-value4"
  key5 = "${local.key4}-value5"
}
`
	require.NoError(tb, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
}

// writeLiveLayoutUnits spreads units evenly across envDirs as
// unitNNNNN/terragrunt.hcl. Every third unit within an env directory depends
// on the unit created immediately before it.
func writeLiveLayoutUnits(tb testing.TB, envDirs []string, units int) {
	tb.Helper()

	perEnv := units / len(envDirs)
	unitIdx := 0

	for _, envDir := range envDirs {
		prevUnitName := ""

		for i := range perEnv {
			unitName := fmt.Sprintf("unit%05d", unitIdx)
			unitDir := filepath.Join(envDir, unitName)
			require.NoError(tb, os.MkdirAll(unitDir, 0o755))

			dependency := ""
			if i%3 == 2 {
				dependency = prevUnitName
			}

			require.NoError(
				tb,
				os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(liveLayoutUnitHCL(dependency)), 0o644),
			)

			prevUnitName = unitName
			unitIdx++
		}
	}
}

// liveLayoutUnitHCL renders a live layout unit that includes root.hcl. A
// non-empty dependency names a sibling unit the rendered one depends on,
// with mock outputs, and reads in its inputs.
func liveLayoutUnitHCL(dependency string) string {
	const header = "include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n}\n\n" +
		"terraform {\n  source = \"./module\"\n}\n\n"

	if dependency == "" {
		return header + "inputs = {\n  name  = \"unit\"\n  count = 1\n}\n"
	}

	return header + fmt.Sprintf(
		"dependency \"prev\" {\n  config_path = \"../%s\"\n\n  mock_outputs = {\n    id = \"x\"\n  }\n}\n\n",
		dependency,
	) + "inputs = {\n  name   = \"unit\"\n  count  = 1\n  dep_id = dependency.prev.outputs.id\n}\n"
}
