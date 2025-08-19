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

func BenchmarkSixLevelDependenciesProject(b *testing.B) {
	emptyMainTf := `resource "null_resource" "test" {
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

	// Create a temporary directory for the test
	tmpDir := b.TempDir()
	rootTerragruntConfigPath := filepath.Join(tmpDir, "root.hcl")
	// Create an empty `root.hcl` file
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), helpers.DefaultFilePermissions))

	// Create the 6-level dependency structure:
	// app1 -> dep5 -> dep4 -> dep3 -> dep2 -> dep1 -> common
	// app2 -> dep3 -> dep2 -> dep1 -> common
	// app3 -> dep1 -> common

	// Create common module (level 0)
	commonDir := filepath.Join(tmpDir, "common")
	require.NoError(b, os.MkdirAll(commonDir, helpers.DefaultDirPermissions))
	commonTerragruntConfigPath := filepath.Join(commonDir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(commonTerragruntConfigPath, []byte(includeRootConfig), helpers.DefaultFilePermissions))
	commonMainTfPath := filepath.Join(commonDir, "main.tf")
	require.NoError(b, os.WriteFile(commonMainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create dep1 module (level 1, depends on common)
	dep1Dir := filepath.Join(tmpDir, "dep1")
	require.NoError(b, os.MkdirAll(dep1Dir, helpers.DefaultDirPermissions))
	dep1TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../common"]
}`
	dep1TerragruntConfigPath := filepath.Join(dep1Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(dep1TerragruntConfigPath, []byte(dep1TerragruntConfig), helpers.DefaultFilePermissions))
	dep1MainTfPath := filepath.Join(dep1Dir, "main.tf")
	require.NoError(b, os.WriteFile(dep1MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create dep2 module (level 2, depends on dep1)
	dep2Dir := filepath.Join(tmpDir, "dep2")
	require.NoError(b, os.MkdirAll(dep2Dir, helpers.DefaultDirPermissions))
	dep2TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep1"]
}`
	dep2TerragruntConfigPath := filepath.Join(dep2Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(dep2TerragruntConfigPath, []byte(dep2TerragruntConfig), helpers.DefaultFilePermissions))
	dep2MainTfPath := filepath.Join(dep2Dir, "main.tf")
	require.NoError(b, os.WriteFile(dep2MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create dep3 module (level 3, depends on dep2)
	dep3Dir := filepath.Join(tmpDir, "dep3")
	require.NoError(b, os.MkdirAll(dep3Dir, helpers.DefaultDirPermissions))
	dep3TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep2"]
}`
	dep3TerragruntConfigPath := filepath.Join(dep3Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(dep3TerragruntConfigPath, []byte(dep3TerragruntConfig), helpers.DefaultFilePermissions))
	dep3MainTfPath := filepath.Join(dep3Dir, "main.tf")
	require.NoError(b, os.WriteFile(dep3MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create dep4 module (level 4, depends on dep3)
	dep4Dir := filepath.Join(tmpDir, "dep4")
	require.NoError(b, os.MkdirAll(dep4Dir, helpers.DefaultDirPermissions))
	dep4TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep3"]
}`
	dep4TerragruntConfigPath := filepath.Join(dep4Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(dep4TerragruntConfigPath, []byte(dep4TerragruntConfig), helpers.DefaultFilePermissions))
	dep4MainTfPath := filepath.Join(dep4Dir, "main.tf")
	require.NoError(b, os.WriteFile(dep4MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create dep5 module (level 5, depends on dep4)
	dep5Dir := filepath.Join(tmpDir, "dep5")
	require.NoError(b, os.MkdirAll(dep5Dir, helpers.DefaultDirPermissions))
	dep5TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep4"]
}`
	dep5TerragruntConfigPath := filepath.Join(dep5Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(dep5TerragruntConfigPath, []byte(dep5TerragruntConfig), helpers.DefaultFilePermissions))
	dep5MainTfPath := filepath.Join(dep5Dir, "main.tf")
	require.NoError(b, os.WriteFile(dep5MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create app1 module (level 6, depends on dep5)
	app1Dir := filepath.Join(tmpDir, "app1")
	require.NoError(b, os.MkdirAll(app1Dir, helpers.DefaultDirPermissions))
	app1TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep5"]
}`
	app1TerragruntConfigPath := filepath.Join(app1Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(app1TerragruntConfigPath, []byte(app1TerragruntConfig), helpers.DefaultFilePermissions))
	app1MainTfPath := filepath.Join(app1Dir, "main.tf")
	require.NoError(b, os.WriteFile(app1MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create app2 module (level 4, depends on dep3)
	app2Dir := filepath.Join(tmpDir, "app2")
	require.NoError(b, os.MkdirAll(app2Dir, helpers.DefaultDirPermissions))
	app2TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep3"]
}`
	app2TerragruntConfigPath := filepath.Join(app2Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(app2TerragruntConfigPath, []byte(app2TerragruntConfig), helpers.DefaultFilePermissions))
	app2MainTfPath := filepath.Join(app2Dir, "main.tf")
	require.NoError(b, os.WriteFile(app2MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Create app3 module (level 2, depends on dep1)
	app3Dir := filepath.Join(tmpDir, "app3")
	require.NoError(b, os.MkdirAll(app3Dir, helpers.DefaultDirPermissions))
	app3TerragruntConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependencies {
	paths = ["../dep1"]
}`
	app3TerragruntConfigPath := filepath.Join(app3Dir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(app3TerragruntConfigPath, []byte(app3TerragruntConfig), helpers.DefaultFilePermissions))
	app3MainTfPath := filepath.Join(app3Dir, "main.tf")
	require.NoError(b, os.WriteFile(app3MainTfPath, []byte(emptyMainTf), helpers.DefaultFilePermissions))

	// Do an initial init to avoid noise from the first iteration being slower
	helpers.Init(b, tmpDir)

	b.Run("default_runner", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < 10; i++ {
			helpers.Apply(b, tmpDir)
		}
	})

	b.Run("runner_pool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < 10; i++ {
			helpers.ApplyWithRunnerPool(b, tmpDir)
		}
	})
}
