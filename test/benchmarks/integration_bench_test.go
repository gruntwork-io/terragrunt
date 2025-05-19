package test_test

import (
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

	b.Run("1 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for the test
			tmpDir := b.TempDir()
			rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
			// Create an empty `root.hcl` file
			require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

			// Create 10 units
			helpers.GenerateNUnits(b, tmpDir, 1, includeRootConfig, emptyMainTf)

			b.StartTimer()

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

	b.Run("1 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for the test
			tmpDir := b.TempDir()
			rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
			// Create an empty `root.hcl` file
			require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

			// Create 10 units
			helpers.GenerateNUnits(b, tmpDir, 1, includeRootConfig, emptyMainTf)

			b.StartTimer()

			helpers.Plan(b, tmpDir)
		}
	})

}
