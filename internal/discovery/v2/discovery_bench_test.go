package v2_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	v2 "github.com/gruntwork-io/terragrunt/internal/discovery/v2"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	logformat "github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

// BenchmarkFixture holds the generated fixture directory and metadata.
type BenchmarkFixture struct {
	RootDir        string
	ComponentCount int
}

func BenchmarkDiscoveryV1vsV2(b *testing.B) {
	sizes := []int{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			fixture := generateBenchmarkFixture(b, size)
			l := createTestLogger()
			opts := createTestOpts(b, fixture.RootDir)

			// FilesystemOnly - basic directory walking
			b.Run("FilesystemOnly", func(b *testing.B) {
				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithParsing - parse phase enabled
			b.Run("WithParsing", func(b *testing.B) {
				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithRequiresParse().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithRequiresParse().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithParseExclude - parse + exclude blocks
			b.Run("WithParseExclude", func(b *testing.B) {
				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithParseExclude().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithParseExclude().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithRelationships - relationship discovery phase
			b.Run("WithRelationships", func(b *testing.B) {
				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithRelationships().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithRelationships().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithDependencyFilter - graph phase (deps) with filter like {./apps/api}...
			b.Run("WithDependencyFilter", func(b *testing.B) {
				filterStr := "{./apps/api}..."
				filters, err := filter.ParseFilterQueries(l, []string{filterStr})
				require.NoError(b, err)

				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithDependentFilter - graph phase (dependents) with filter like ..../vpc
			b.Run("WithDependentFilter", func(b *testing.B) {
				filterStr := "..../vpc"
				filters, err := filter.ParseFilterQueries(l, []string{filterStr})
				require.NoError(b, err)

				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithPathFilter - early filtering with ./apps/*
			b.Run("WithPathFilter", func(b *testing.B) {
				filterStr := "./apps/*"
				filters, err := filter.ParseFilterQueries(l, []string{filterStr})
				require.NoError(b, err)

				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithFilters(filters).
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// WithBreakCycles - cycle detection with filters
			b.Run("WithBreakCycles", func(b *testing.B) {
				filterStr := "./cycles/*"
				filters, err := filter.ParseFilterQueries(l, []string{filterStr})
				require.NoError(b, err)

				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithFilters(filters).
							WithBreakCycles().
							WithRelationships().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithFilters(filters).
							WithBreakCycles().
							WithRelationships().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})

			// FullPipeline - all options enabled
			b.Run("FullPipeline", func(b *testing.B) {
				filterStr := "./apps/*"
				filters, err := filter.ParseFilterQueries(l, []string{filterStr})
				require.NoError(b, err)

				b.Run("v1", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := discovery.NewDiscovery(fixture.RootDir).
							WithFilters(filters).
							WithRequiresParse().
							WithParseExclude().
							WithRelationships().
							WithBreakCycles().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
				b.Run("v2", func(b *testing.B) {
					b.ResetTimer()

					for b.Loop() {
						d := v2.New(fixture.RootDir).
							WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fixture.RootDir}).
							WithFilters(filters).
							WithRequiresParse().
							WithParseExclude().
							WithRelationships().
							WithBreakCycles().
							WithSuppressParseErrors()
						components, err := d.Discover(b.Context(), l, opts)
						require.NoError(b, err)
						b.ReportMetric(float64(len(components)), "components/op")
					}
				})
			})
		})
	}
}

// generateBenchmarkFixture creates a fixture directory structure for benchmarking.
// The structure exercises all discovery types:
//
//	/fixture
//	├── root.hcl                    # Empty root config for include
//	├── shared/common.hcl           # Read by units (triggers parse/reading)
//	├── vpc/terragrunt.hcl          # Base unit (no dependencies)
//	├── db/terragrunt.hcl           # Depends on vpc
//	├── cache/terragrunt.hcl        # Depends on vpc
//	├── apps/
//	│   ├── api/terragrunt.hcl      # Depends on db, cache (diamond)
//	│   ├── worker/terragrunt.hcl   # Depends on db, cache (diamond)
//	│   └── web-{N}/terragrunt.hcl  # Linear chain off api
//	├── independent-{N}/terragrunt.hcl  # Units with exclude blocks
//	├── cycles/
//	│   ├── foo/terragrunt.hcl      # Depends on bar
//	│   └── bar/terragrunt.hcl      # Depends on foo
//	└── stacks/terragrunt.stack.hcl # Stack file
func generateBenchmarkFixture(b *testing.B, componentCount int) *BenchmarkFixture {
	b.Helper()

	rootDir := b.TempDir()

	// Initialize the Git repo
	g, err := git.NewGitRunner()
	require.NoError(b, err)

	g = g.WithWorkDir(rootDir)

	err = g.Init(b.Context())
	require.NoError(b, err)

	// Create root.hcl for includes
	createBenchUnit(b, rootDir, "root.hcl", rootConfig())

	// Create shared config directory
	sharedDir := filepath.Join(rootDir, "shared")
	require.NoError(b, os.MkdirAll(sharedDir, 0755))
	require.NoError(b, os.WriteFile(filepath.Join(sharedDir, "common.hcl"), []byte(sharedConfig()), 0644))

	// Create base infrastructure units
	createBenchUnit(b, rootDir, "vpc", baseUnitConfig())
	createBenchUnit(b, rootDir, "db", unitWithDeps(b, "../vpc"))
	createBenchUnit(b, rootDir, "cache", unitWithDeps(b, "../vpc"))

	// Create apps directory with diamond dependency pattern
	appsDir := filepath.Join(rootDir, "apps")
	require.NoError(b, os.MkdirAll(appsDir, 0755))

	createBenchUnit(b, appsDir, "api", unitWithDeps(b, "../../db", "../../cache"))
	createBenchUnit(b, appsDir, "worker", unitWithDeps(b, "../../db", "../../cache"))

	// Create web-N units forming a linear chain off api
	// Number of web units scales with componentCount
	webCount := max(componentCount/4, 1)

	prevDep := "../api"

	for i := 1; i <= webCount; i++ {
		unitName := fmt.Sprintf("web-%d", i)
		createBenchUnit(b, appsDir, unitName, unitWithDeps(b, prevDep))
		prevDep = "../" + unitName
	}

	// Create independent units with exclude blocks
	// Number scales with componentCount
	independentCount := max(componentCount/4, 1)

	for i := 1; i <= independentCount; i++ {
		unitName := fmt.Sprintf("independent-%d", i)
		createBenchUnit(b, rootDir, unitName, unitWithExclude)
	}

	// Create cycles directory with circular dependencies
	cyclesDir := filepath.Join(rootDir, "cycles")
	require.NoError(b, os.MkdirAll(cyclesDir, 0755))

	createBenchUnit(b, cyclesDir, "foo", unitWithDeps(b, "../bar"))
	createBenchUnit(b, cyclesDir, "bar", unitWithDeps(b, "../foo"))

	// Create stacks directory with stack file
	stacksDir := filepath.Join(rootDir, "stacks")
	require.NoError(b, os.MkdirAll(stacksDir, 0755))
	require.NoError(b, os.WriteFile(filepath.Join(stacksDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0644))

	// Add additional units to reach target componentCount
	// Current count: vpc(1) + db(1) + cache(1) + api(1) + worker(1) + web-N(webCount) +
	//                independent-N(independentCount) + cycles/foo(1) + cycles/bar(1) + stack(1)
	currentCount := 1 + 1 + 1 + 1 + 1 + webCount + independentCount + 1 + 1 + 1
	extraCount := componentCount - currentCount

	if extraCount > 0 {
		extraDir := filepath.Join(rootDir, "extra")
		require.NoError(b, os.MkdirAll(extraDir, 0755))

		for i := 1; i <= extraCount; i++ {
			unitName := fmt.Sprintf("extra-%d", i)
			// Vary dependency patterns
			switch i % 3 {
			case 0:
				createBenchUnit(b, extraDir, unitName, unitWithDeps(b, "../../vpc"))
			case 1:
				createBenchUnit(b, extraDir, unitName, unitWithDeps(b, "../../db"))
			default:
				createBenchUnit(b, extraDir, unitName, baseUnitConfig())
			}
		}
	}

	return &BenchmarkFixture{
		RootDir:        rootDir,
		ComponentCount: componentCount,
	}
}

// createBenchUnit creates a terragrunt.hcl file in a subdirectory.
func createBenchUnit(b *testing.B, baseDir, name, content string) string {
	b.Helper()

	// If name has .hcl extension, create file directly
	if filepath.Ext(name) == ".hcl" {
		filePath := filepath.Join(baseDir, name)
		require.NoError(b, os.WriteFile(filePath, []byte(content), 0644))

		return filePath
	}

	// Otherwise create directory with terragrunt.hcl
	dirPath := filepath.Join(baseDir, name)
	require.NoError(b, os.MkdirAll(dirPath, 0755))

	filePath := filepath.Join(dirPath, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(filePath, []byte(content), 0644))

	return dirPath
}

// rootConfig returns the content for root.hcl.
func rootConfig() string {
	return `# Root configuration for includes
locals {
  root_var = "root"
}
`
}

// sharedConfig returns the content for shared/common.hcl.
func sharedConfig() string {
	return `# Shared configuration
locals {
  shared_var = "shared"
  environment = "benchmark"
}
`
}

// baseUnitConfig returns a basic terragrunt.hcl without dependencies.
func baseUnitConfig() string {
	return `# Base unit configuration
locals {
  name = "base-unit"
}

terraform {
  source = "../../modules//vpc"
}
`
}

// unitWithDeps returns a terragrunt.hcl with dependency blocks.
func unitWithDeps(b *testing.B, deps ...string) string {
	b.Helper()

	config := `# Unit with dependencies
locals {
  name = "dependent-unit"
}

terraform {
  source = "../../modules//app"
}

`

	var configSb487 strings.Builder

	for i, dep := range deps {
		n, err := fmt.Fprintf(
			&configSb487,
			`dependency "dep%d" {
  config_path = "%s"
}

`,
			i,
			dep)
		require.NoError(b, err)
		require.Positive(b, n)
	}

	config += configSb487.String()

	return config
}

const unitWithExclude = `# Unit with exclude block
locals {
  name = "excluded-unit"
}

terraform {
  source = "../modules//independent"
}

exclude {
  if = false
  actions = ["all"]
}
`

const stackConfig = `# Stack configuration
stack {
  name = "benchmark-stack"
}

unit "vpc" {
  source = "../vpc"
}

unit "db" {
  source = "../db"
}
`

// createTestLogger creates a logger configured for benchmarks with output discarded.
func createTestLogger() log.Logger {
	formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(
		log.WithOutput(io.Discard),
		log.WithLevel(log.ErrorLevel),
		log.WithFormatter(formatter),
	)
}

// createTestOpts creates TerragruntOptions configured for benchmarks.
func createTestOpts(b *testing.B, workingDir string) *options.TerragruntOptions {
	b.Helper()

	opts, err := options.NewTerragruntOptionsForTest(workingDir)
	require.NoError(b, err)

	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir
	opts.Writer = io.Discard
	opts.ErrWriter = io.Discard

	return opts
}
