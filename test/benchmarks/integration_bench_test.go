package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

func BenchmarkEmptyTerragruntPlanApply(b *testing.B) {
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}
`

	b.Run("10 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for the test
			tmpDir := b.TempDir()
			rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
			// Create an empty `root.hcl` file
			require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), 0644))

			// Create 10 units
			helpers.GenerateNUnits(b, tmpDir, 10, includeRootConfig, emptyMainTf)

			b.StartTimer()

			helpers.PlanApplyDestroy(b, tmpDir)
		}
	})

	b.Run("100 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for the test
			tmpDir := b.TempDir()
			rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")

			// Create an empty `root.hcl` file
			require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), 0644))

			// Create 100 units
			helpers.GenerateNUnits(b, tmpDir, 100, includeRootConfig, emptyMainTf)

			b.StartTimer()

			helpers.PlanApplyDestroy(b, tmpDir)
		}
	})

	b.Run("1000 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for the test
			tmpDir := b.TempDir()
			rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")

			// Create an empty `root.hcl` file
			require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), 0644))

			// Create 1000 units
			helpers.GenerateNUnits(b, tmpDir, 1000, includeRootConfig, emptyMainTf)

			b.StartTimer()

			helpers.PlanApplyDestroy(b, tmpDir)
		}
	})
}

// TODO: Enable this once it's fixed.
//
// func BenchmarkDependencyTrainPlanApply(b *testing.B) {
// 	b.Run("10 units", func(b *testing.B) {
// 		for i := 0; i < b.N; i++ {
// 			b.StopTimer()
//
// 			tmpDir := b.TempDir()
// 			helpers.GenerateDependencyTrain(b, tmpDir, 10)
//
// 			b.StartTimer()
//
// 			helpers.PlanApplyDestroy(b, tmpDir)
// 		}
// 	})
//
// 	b.Run("100 units", func(b *testing.B) {
// 		for i := 0; i < b.N; i++ {
// 			b.StopTimer()
//
// 			tmpDir := b.TempDir()
// 			helpers.GenerateDependencyTrain(b, tmpDir, 100)
//
// 			b.StartTimer()
//
// 			helpers.PlanApplyDestroy(b, tmpDir)
// 		}
// 	})
//
// 	b.Run("1000 units", func(b *testing.B) {
// 		for i := 0; i < b.N; i++ {
// 			b.StopTimer()
//
// 			tmpDir := b.TempDir()
// 			helpers.GenerateDependencyTrain(b, tmpDir, 1000)
//
// 			b.StartTimer()
//
// 			helpers.PlanApplyDestroy(b, tmpDir)
// 		}
// 	})
// }
