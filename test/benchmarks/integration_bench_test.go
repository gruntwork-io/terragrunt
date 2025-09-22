package test_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

// warmupApplies performs a number of unmeasured apply runs to warm caches and workers.
func warmupApplies(b *testing.B, tmpDir string, useRunnerPool bool, count int) {
	b.Helper()

	for i := 0; i < count; i++ {
		if useRunnerPool {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		} else {
			helpers.Apply(b, tmpDir)
		}
	}
}

func BenchmarkEmptyTerragruntInit(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}
`

	// Create a temporary directory for the test
	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	// Create an empty `root.hcl` file
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Create 1 units
	helpers.GenerateNUnits(b, tmpDir, 1, includeRootConfig, emptyMainTf)

	// Do an initial init to avoid noise from the first iteration being slower
	helpers.Init(b, tmpDir)

	b.Run("1 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Init(b, tmpDir)
		}
	})
}

func BenchmarkTwoEmptyTerragruntInits(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}
`

	tmpDir := b.TempDir()

	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	helpers.GenerateNUnits(b, tmpDir, 2, includeRootConfig, emptyMainTf)

	// Do an initial init to avoid noise from the first iteration being slower
	helpers.Init(b, tmpDir)

	b.Run("2 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Init(b, tmpDir)
		}
	})
}

func BenchmarkManyEmptyTerragruntInits(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}
`

	tmpDir := b.TempDir()

	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	helpers.GenerateNUnits(b, tmpDir, 1000, includeRootConfig, emptyMainTf)

	// Do an initial init to avoid noise from the first iteration being slower
	helpers.Init(b, tmpDir)

	b.Run("1000 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Init(b, tmpDir)
		}
	})
}

func BenchmarkEmptyTerragruntPlan(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}
`

	// Create a temporary directory for the test
	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	// Create an empty `root.hcl` file
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Create 1 units
	helpers.GenerateNUnits(b, tmpDir, 1, includeRootConfig, emptyMainTf)

	helpers.Init(b, tmpDir)

	b.Run("1 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Plan(b, tmpDir)
		}
	})
}

func BenchmarkTwoEmptyTerragruntPlans(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
	}

	terraform {
		source = "."
	}
`

	tmpDir := b.TempDir()

	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	helpers.GenerateNUnits(b, tmpDir, 2, includeRootConfig, emptyMainTf)

	helpers.Init(b, tmpDir)

	b.Run("2 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Plan(b, tmpDir)
		}
	})
}

func BenchmarkManyEmptyTerragruntPlans(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
	}

	terraform {
		source = "."
	}
`

	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	helpers.GenerateNUnits(b, tmpDir, 1000, includeRootConfig, emptyMainTf)

	helpers.Init(b, tmpDir)

	b.Run("1000 units", func(b *testing.B) {
		for b.Loop() {
			helpers.Plan(b, tmpDir)
		}
	})
}

func BenchmarkUnitsNoDependencies(b *testing.B) {
	baseMainTf := `resource "null_resource" "test" {
  triggers = {
    timestamp = timestamp()
  }
}`

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
        path = find_in_parent_folders("root.hcl")
}
terraform {
    source = "."
}
`

	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	helpers.GenerateNUnits(b, tmpDir, 10, includeRootConfig, baseMainTf)

	helpers.Init(b, tmpDir)

	b.Run("default_runner", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, false, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		}
	})
}

func BenchmarkUnitsNoDependenciesRandomWait(b *testing.B) {
	emptyRootConfig := ``
	includeRootConfig := `include "root" {
        path = find_in_parent_folders("root.hcl")
}
terraform {
    source = "."
}
`

	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Generate independent units with random 100-300ms waits
	for i := 0; i < 10; i++ {
		unitDir := filepath.Join(tmpDir, fmt.Sprintf("unit-%d", i))
		require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

		tgPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(tgPath, []byte(includeRootConfig), helpers.DefaultFilePermissions))

		ms := 100 + rand.Intn(201) // 100..300 ms
		secs := float64(ms) / 1000.0
		mainTf := fmt.Sprintf(`resource "null_resource" "wait" {
  provisioner "local-exec" {
    command = "bash -c 'sleep %.3f'"
  }
  triggers = {
    timestamp = timestamp()
  }
}
`, secs)
		tfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(tfPath, []byte(mainTf), helpers.DefaultFilePermissions))
	}

	helpers.Init(b, tmpDir)

	b.Run("default_runner", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, false, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		}
	})
}

func BenchmarkUnitsOneDependencyWithWait(b *testing.B) {
	baseMainTf := `resource "null_resource" "test" {
  triggers = {
    timestamp = timestamp()
  }
}`

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
        path = find_in_parent_folders("root.hcl")
}
terraform {
    source = "."
}
`

	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Create units
	for i := 0; i < 10; i++ {
		unitDir := filepath.Join(tmpDir, fmt.Sprintf("unit-%d", i))
		require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

		// terragrunt.hcl
		var tgConfig string
		if i == 2 {
			// unit-2 depends on unit-1
			tgConfig = `include "root" {
        path = find_in_parent_folders("root.hcl")
}
terraform {
    source = "."
}
dependencies {
    paths = ["../unit-1"]
}`
		} else {
			tgConfig = includeRootConfig
		}

		tgPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(tgPath, []byte(tgConfig), helpers.DefaultFilePermissions))

		// main.tf
		var tfConfig string
		if i == 1 {
			// unit-1 has 400ms wait
			tfConfig = `resource "null_resource" "wait" {
  provisioner "local-exec" {
    command = "bash -c 'sleep 0.4'"
  }
  triggers = {
    timestamp = timestamp()
  }
}`
		} else {
			tfConfig = baseMainTf
		}

		tfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(tfPath, []byte(tfConfig), helpers.DefaultFilePermissions))
	}

	helpers.Init(b, tmpDir)

	b.Run("default_runner", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, false, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for i := 0; i < 10; i++ {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		}
	})
}

// BenchmarkDependencyFanInOddDependsOnEvenRandomWait generates N units (2,4,8,...,128) where:
// - Every odd-indexed unit depends on every even-indexed unit
// - Each unit performs a random sleep via local-exec to simulate workload
// It measures apply times for both the default runner (configstack) and the runner pool.
func BenchmarkDependencyFanInOddDependsOnEvenRandomWait(b *testing.B) {
	// Sizes for parameterized benchmark
	sizes := []int{2, 4, 8, 16, 32, 64, 128}

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}
terraform {
	source = "."
}
`

	for _, n := range sizes {
		b.Run(fmt.Sprintf("%d_units", n), func(b *testing.B) {
			// Generate two variants: slow (with delays) for configstack, fast (no delays) for runner_pool
			slowDir := b.TempDir()
			fastDir := b.TempDir()

			// Write root.hcl for both variants
			require.NoError(b, os.WriteFile(filepath.Join(slowDir, "root.hcl"), []byte(emptyRootConfig), helpers.DefaultFilePermissions))
			require.NoError(b, os.WriteFile(filepath.Join(fastDir, "root.hcl"), []byte(emptyRootConfig), helpers.DefaultFilePermissions))

			// Helper to generate a variant
			generate := func(baseDir string, withDelay bool) {
				for i := 0; i < n; i++ {
					unitDir := filepath.Join(baseDir, fmt.Sprintf("unit-%d", i))
					require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

					// terragrunt.hcl: odd units depend on even units via dependency blocks
					var tgConfig string
					if i%2 == 1 { // odd unit depends on every even unit
						deps := make([]string, 0, (n+1)/2)
						for j := 0; j < n; j++ {
							if j%2 == 0 { // even
								deps = append(deps, fmt.Sprintf("../unit-%d", j))
							}
						}
						// Build multiple dependency blocks
						var depBlocks []string
						for _, p := range deps {
							name := strings.ReplaceAll(filepath.Base(p), "-", "_")
							depBlocks = append(depBlocks, fmt.Sprintf("dependency \"%s\" {\n  config_path = \"%s\"\n}\n", name, p))
						}
						tgConfig = includeRootConfig + strings.Join(depBlocks, "\n")
					} else {
						tgConfig = includeRootConfig
					}

					tgPath := filepath.Join(unitDir, "terragrunt.hcl")
					require.NoError(b, os.WriteFile(tgPath, []byte(tgConfig), helpers.DefaultFilePermissions))

					// main.tf: withDelay => dependents (odd units) sleep random 20-50ms; providers (even) no sleep; without => noop
					var mainTf string
					var secs float64
					if withDelay && i%2 == 1 { // only dependents get delay
						ms := 20 + rand.Intn(31) // 20..50ms
						secs = float64(ms) / 1000.0
					}

					if i%2 == 0 { // even unit: define output sleep_seconds
						if withDelay {
							// No delay for providers in slow stack; only expose output=0
							mainTf = `resource "null_resource" "noop" {
  triggers = {
    timestamp = timestamp()
  }
}

output "sleep_seconds" {
  value = 0
}
`
						} else {
							mainTf = `resource "null_resource" "noop" {
  triggers = {
    timestamp = timestamp()
  }
}

output "sleep_seconds" {
  value = 0
}
`
						}
					} else {
						if withDelay {
							mainTf = fmt.Sprintf(`resource "null_resource" "wait" {
  provisioner "local-exec" {
    command = "bash -c 'sleep %.3f'"
  }
  triggers = {
    timestamp = timestamp()
  }
}
`, secs)
						} else {
							mainTf = `resource "null_resource" "noop" {
  triggers = {
    timestamp = timestamp()
  }
}
`
						}
					}
					tfPath := filepath.Join(unitDir, "main.tf")
					require.NoError(b, os.WriteFile(tfPath, []byte(mainTf), helpers.DefaultFilePermissions))
				}
			}

			// Generate both variants
			generate(slowDir, true)
			generate(fastDir, false)

			// Init once to prepare
			helpers.Init(b, slowDir)
			helpers.Init(b, fastDir)

			b.Run("configstack", func(b *testing.B) {
				// Warmups (not measured)
				warmupApplies(b, slowDir, false, 2)
				b.ResetTimer()

				for i := 0; i < 10; i++ {
					helpers.Apply(b, slowDir)
				}
			})

			b.Run("runner_pool", func(b *testing.B) {
				// Warmups (not measured)
				warmupApplies(b, fastDir, true, 2)
				b.ResetTimer()

				for i := 0; i < 10; i++ {
					helpers.ApplyWithRunnerPool(b, fastDir)
				}
			})
		})
	}
}
