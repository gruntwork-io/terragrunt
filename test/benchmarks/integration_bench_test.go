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

	// Create a temporary directory for the test
	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	// Create an empty `root.hcl` file
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Create 1 units
	helpers.GenerateNUnits(b, tmpDir, 1, includeRootConfig, emptyMainTf)

	b.Run("1 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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

	b.Run("2 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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

	b.Run("1000 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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

	b.Run("1 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {

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

	b.Run("2 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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

	b.Run("1000 units", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			helpers.Plan(b, tmpDir)
		}
	})
}
