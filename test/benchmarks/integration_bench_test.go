package test_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

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

		for b.Loop() {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for b.Loop() {
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

		for b.Loop() {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for b.Loop() {
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

		for b.Loop() {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		// Warmups (not measured)
		warmupApplies(b, tmpDir, true, 2)
		b.ResetTimer()

		for b.Loop() {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		}
	})
}

// BenchmarkDependencyPairwiseOddDependsOnPrevEvenRandomWait generates N units (50, 100) where:
// - Every odd-indexed unit depends on the previous even-indexed unit (e.g., 1->0, 3->2, ...)
// - Even-indexed units perform a random sleep via local-exec to simulate workload (50..100ms)
// - Odd-indexed units are no-ops but depend on their paired even unit
// It measures apply times for both the default runner (configstack) and the runner pool on the SAME stack.
func BenchmarkDependencyPairwiseOddDependsOnPrevEvenRandomWait(b *testing.B) {
	// Sizes for parameterized benchmark (2,4,8,...,128)
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
			// Generate a single stack used by both runners
			dir := b.TempDir()

			// Write root.hcl
			require.NoError(b, os.WriteFile(filepath.Join(dir, "root.hcl"), []byte(emptyRootConfig), helpers.DefaultFilePermissions))

			// Seed random generator deterministically within this sub-benchmark
			rnd := rand.New(rand.NewSource(int64(n)))

			// Generate units where every odd depends on every even
			for i := 0; i < n; i++ {
				unitDir := filepath.Join(dir, fmt.Sprintf("unit-%d", i))
				require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

				// terragrunt.hcl: odd units depend only on the previous even unit (i-1)
				var tgConfig string

				if i%2 == 1 {
					prev := i - 1
					if prev >= 0 {
						depBlock := fmt.Sprintf("dependency \"unit_%d\" {\n  config_path = \"../unit-%d\"\n}\n\n", prev, prev)
						tgConfig = includeRootConfig + depBlock
					} else {
						tgConfig = includeRootConfig
					}
				} else {
					tgConfig = includeRootConfig
				}

				tgPath := filepath.Join(unitDir, "terragrunt.hcl")
				require.NoError(b, os.WriteFile(tgPath, []byte(tgConfig), helpers.DefaultFilePermissions))

				// main.tf: even units wait 50..100ms; odd units are no-ops
				var mainTf string

				if i%2 == 0 { // even: random sleep
					ms := 50 + rnd.Intn(51) // 50..100ms
					secs := float64(ms) / 1000.0
					mainTf = fmt.Sprintf(`resource "null_resource" "wait" {
  provisioner "local-exec" {
    command = "bash -c 'sleep %.3f'"
  }
  triggers = {
    timestamp = timestamp()
  }
}
`, secs)
				} else { // odd: noop
					mainTf = `resource "null_resource" "noop" {
  triggers = {
    timestamp = timestamp()
  }
}
`
				}

				tfPath := filepath.Join(unitDir, "main.tf")
				require.NoError(b, os.WriteFile(tfPath, []byte(mainTf), helpers.DefaultFilePermissions))
			}

			// Init once to prepare
			helpers.Init(b, dir)

			b.Run("configstack", func(b *testing.B) {
				// Warmups (not measured)
				warmupApplies(b, dir, false, 2)
				b.ResetTimer()

				for b.Loop() {
					helpers.Apply(b, dir)
				}
			})

			b.Run("runner_pool", func(b *testing.B) {
				// Warmups (not measured)
				warmupApplies(b, dir, true, 2)
				b.ResetTimer()

				for b.Loop() {
					helpers.ApplyWithRunnerPool(b, dir)
				}
			})
		})
	}
}

// warmupApplies performs a number of unmeasured apply runs to warm caches and workers.
func warmupApplies(b *testing.B, tmpDir string, useRunnerPool bool, count int) {
	b.Helper()

	for range make([]struct{}, count) {
		if useRunnerPool {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		} else {
			helpers.Apply(b, tmpDir)
		}
	}
}
