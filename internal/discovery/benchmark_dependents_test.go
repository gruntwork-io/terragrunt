package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// dependentUnitCounts sizes the dependents fixtures: every unit but the target
// depends on the target, so the count is also the size of the selection.
var dependentUnitCounts = []int{10, 50, 200}

// BenchmarkDependentsFilter benchmarks a dependents query against a target that
// every other unit in the fixture depends on. The dependents sit at four
// different directory depths, so the upstream dependent walk climbs through
// directories that hold no config of their own before reaching the units that
// name the target.
func BenchmarkDependentsFilter(b *testing.B) {
	b.Run("from_repo_root", func(b *testing.B) {
		for _, n := range dependentUnitCounts {
			b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
				benchmarkDependents(b, dependentsFromRepoRoot, fanInDependentsFixture(n))
			})
		}
	})

	b.Run("from_target_dir", func(b *testing.B) {
		for _, n := range dependentUnitCounts {
			b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
				benchmarkDependents(b, dependentsFromTargetDir, fanInDependentsFixture(n))
			})
		}
	})
}

// fanInDependentsFixture builds a fixture of n units in which every unit but the
// target depends on the target, so the query selects the whole fixture.
func fanInDependentsFixture(n int) dependentsFixture {
	return func(tb testing.TB, dir string) (string, int) {
		tb.Helper()

		return createDependentsFixture(tb, dir, n), n
	}
}

// createDependentsFixture writes n units under tmpDir and returns the target's
// path. One unit is the target; the other n-1 each declare a dependency on it,
// nested one to four directories deep under svc/.
func createDependentsFixture(tb testing.TB, tmpDir string, n int) string {
	tb.Helper()

	target := filepath.Join(tmpDir, "vpc")
	require.NoError(tb, os.MkdirAll(target, 0755))
	require.NoError(tb, os.WriteFile(
		filepath.Join(target, "terragrunt.hcl"),
		[]byte("# Target unit\n"),
		0644,
	))

	for i := range n - 1 {
		dir := filepath.Join(tmpDir, "svc")

		for level := range i%4 + 1 {
			dir = filepath.Join(dir, fmt.Sprintf("level-%d", level))
		}

		dir = filepath.Join(dir, fmt.Sprintf("dependent-%04d", i))
		require.NoError(tb, os.MkdirAll(dir, 0755))

		rel, err := filepath.Rel(dir, target)
		require.NoError(tb, err)

		require.NoError(tb, os.WriteFile(
			filepath.Join(dir, "terragrunt.hcl"),
			fmt.Appendf(nil, "dependency \"vpc\" {\n  config_path = %q\n}\n", rel),
			0644,
		))
	}

	return target
}
