package discovery_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

// unitCounts defines geometric scaling for benchmark fixture sizes.
var unitCounts = []int{64, 128, 256, 512, 1024}

func BenchmarkDiscovery(b *testing.B) {
	b.Run("path_expression", func(b *testing.B) {
		for _, n := range unitCounts {
			b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
				benchmarkPathExpression(b, n)
			})
		}
	})

	b.Run("graph_expression", func(b *testing.B) {
		for _, n := range unitCounts {
			b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
				benchmarkGraphExpression(b, n)
			})
		}
	})

	b.Run("path_and_graph_expression", func(b *testing.B) {
		for _, n := range unitCounts {
			b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
				benchmarkPathAndGraphExpression(b, n)
			})
		}
	})
}

// benchmarkPathExpression benchmarks discovery with a path-only filter.
// Targets 2 app units; only filesystem classification runs, no parsing occurs.
func benchmarkPathExpression(b *testing.B, n int) {
	b.Helper()

	tmpDir := b.TempDir()
	createFixtures(b, tmpDir, n)

	l := newDiscardLogger()
	opts := &options.TerragruntOptions{WorkingDir: tmpDir, RootWorkingDir: tmpDir}

	filterQueries, err := filter.ParseFilterQueries(l, []string{"./apps/app-0000", "./apps/app-0001"})
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filterQueries).
			WithSuppressParseErrors()

		components, err := d.Discover(b.Context(), l, opts)
		require.NoError(b, err)
		require.Len(b, components, 2)
	}
}

// benchmarkGraphExpression benchmarks discovery with a graph-only filter.
// Targets a shallow 2-unit dependency pair (infra-0001 → infra-0000).
func benchmarkGraphExpression(b *testing.B, n int) {
	b.Helper()

	tmpDir := b.TempDir()
	createFixtures(b, tmpDir, n)

	l := newDiscardLogger()
	opts := &options.TerragruntOptions{WorkingDir: tmpDir, RootWorkingDir: tmpDir}

	filterQueries, err := filter.ParseFilterQueries(l, []string{"infra-0001..."})
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filterQueries).
			WithSuppressParseErrors()

		components, err := d.Discover(b.Context(), l, opts)
		require.NoError(b, err)
		require.Len(b, components, 2)
	}
}

// benchmarkPathAndGraphExpression benchmarks discovery with combined path + graph filters.
// Targets 2 path-matched apps + 2 graph-traversed infra units (infra-0001 → infra-0000).
func benchmarkPathAndGraphExpression(b *testing.B, n int) {
	b.Helper()

	tmpDir := b.TempDir()
	createFixtures(b, tmpDir, n)

	l := newDiscardLogger()
	opts := &options.TerragruntOptions{WorkingDir: tmpDir, RootWorkingDir: tmpDir}

	filterQueries, err := filter.ParseFilterQueries(l, []string{"./apps/app-0000", "./apps/app-0001", "infra-0001..."})
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filterQueries).
			WithSuppressParseErrors()

		components, err := d.Discover(b.Context(), l, opts)
		require.NoError(b, err)
		require.Len(b, components, 4)
	}
}

func newDiscardLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithOutput(io.Discard), log.WithFormatter(formatter))
}

// createFixtures creates a fixture layout with n total units:
//   - n/2 "app" units in apps/app-NNNN/terragrunt.hcl (minimal, no dependencies)
//   - n/2 "infra" units in infra/infra-NNNN/terragrunt.hcl (paired dependency chains:
//     odd-numbered units depend on the preceding even unit, e.g. infra-0001 → infra-0000)
func createFixtures(b *testing.B, tmpDir string, n int) {
	b.Helper()

	half := n / 2

	appsDir := filepath.Join(tmpDir, "apps")

	for i := range half {
		dir := filepath.Join(appsDir, fmt.Sprintf("app-%04d", i))
		require.NoError(b, os.MkdirAll(dir, 0755))
		require.NoError(b, os.WriteFile(
			filepath.Join(dir, "terragrunt.hcl"),
			[]byte("# Minimal config\n"),
			0644,
		))
	}

	infraDir := filepath.Join(tmpDir, "infra")

	for i := range half {
		dir := filepath.Join(infraDir, fmt.Sprintf("infra-%04d", i))
		require.NoError(b, os.MkdirAll(dir, 0755))

		var content string

		if i%2 == 1 {
			prev := fmt.Sprintf("infra-%04d", i-1)
			content = fmt.Sprintf("dependency \"prev\" {\n  config_path = \"../%s\"\n}\n", prev)
		} else {
			content = "# Leaf unit\n"
		}

		require.NoError(b, os.WriteFile(
			filepath.Join(dir, "terragrunt.hcl"),
			[]byte(content),
			0644,
		))
	}
}
