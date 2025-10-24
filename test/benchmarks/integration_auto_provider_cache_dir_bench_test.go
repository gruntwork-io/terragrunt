package test_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

// BenchmarkAutoProviderCacheDirInit benchmarks Terragrunt init with and without auto provider cache dir enabled
func BenchmarkAutoProviderCacheDirInit(b *testing.B) {
	setup := func(tmpDir string) {
		fixtureSource := filepath.Join("..", "fixtures", "auto-provider-cache-dir", "heavy", "unit")
		terragruntConfigPath := filepath.Join(tmpDir, "terragrunt.hcl")
		mainTfPath := filepath.Join(tmpDir, "main.tf")

		originalTerragruntConfig, err := os.ReadFile(filepath.Join(fixtureSource, "terragrunt.hcl"))
		require.NoError(b, err)

		originalMainTf, err := os.ReadFile(filepath.Join(fixtureSource, "main.tf"))
		require.NoError(b, err)

		require.NoError(b, os.WriteFile(terragruntConfigPath, originalTerragruntConfig, helpers.DefaultFilePermissions))
		require.NoError(b, os.WriteFile(mainTfPath, originalMainTf, helpers.DefaultFilePermissions))

		helpers.RunTerragruntCommand(
			b,
			"terragrunt",
			"init",
			"--non-interactive",
			"--working-dir",
			tmpDir,
		)
	}

	b.Run("init without auto provider cache dir", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		b.ResetTimer()

		for b.Loop() {
			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--source-update",
				"--non-interactive",
				"--working-dir", tmpDir)
		}

		b.StopTimer()
	})

	b.Run("init with auto provider cache dir", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		b.ResetTimer()

		for b.Loop() {
			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--experiment", "auto-provider-cache-dir",
				"--source-update",
				"--non-interactive",
				"--working-dir",
				tmpDir)
		}

		b.StopTimer()
	})
}

// BenchmarkProviderCachingComparison benchmarks Terragrunt init with many units
// comparing no caching, provider cache server, and auto provider cache dir experiment.
func BenchmarkProviderCachingComparison(b *testing.B) {
	setup := func(tmpDir string, count int) {
		fixtureSource := filepath.Join("..", "fixtures", "auto-provider-cache-dir", "heavy", "unit")
		originalTerragruntConfig, err := os.ReadFile(filepath.Join(fixtureSource, "terragrunt.hcl"))
		require.NoError(b, err)
		originalMainTf, err := os.ReadFile(filepath.Join(fixtureSource, "main.tf"))
		require.NoError(b, err)

		// Generate units with the provider configuration
		for i := range count {
			unitDir := filepath.Join(tmpDir, "unit-"+strconv.Itoa(i))
			require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

			unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
			unitMainTfPath := filepath.Join(unitDir, "main.tf")

			require.NoError(b, os.WriteFile(unitTerragruntConfigPath, originalTerragruntConfig, helpers.DefaultFilePermissions))
			require.NoError(b, os.WriteFile(unitMainTfPath, originalMainTf, helpers.DefaultFilePermissions))
		}

		// Run initial init to avoid noise from the first iteration being slower
		helpers.RunTerragruntCommand(
			b,
			"terragrunt",
			"run",
			"--all",
			"init",
			"--non-interactive",
			"--working-dir",
			tmpDir,
		)
	}

	counts := []int{
		1,
		2,
		4,
		8,
		16,
	}

	cacheTypes := []struct {
		name string
		args []string
	}{
		{
			name: "no provider caching",
			args: []string{},
		},
		{
			name: "with provider cache server",
			args: []string{"--provider-cache"},
		},
		{
			name: "with auto provider cache dir",
			args: []string{"--experiment", "auto-provider-cache-dir"},
		},
	}

	for _, count := range counts {
		for _, cacheType := range cacheTypes {
			name := strconv.Itoa(count) + " units " + cacheType.name

			b.Run(name, func(b *testing.B) {
				tmpDir := b.TempDir()

				setup(tmpDir, count)

				args := []string{
					"terragrunt",
					"run",
					"--all",
					"init",
					"--source-update",
					"--non-interactive",
					"--working-dir",
					tmpDir,
				}

				args = append(args, cacheType.args...)

				b.ResetTimer()

				for b.Loop() {
					helpers.RunTerragruntCommand(
						b,
						args...,
					)
				}

				b.StopTimer()
			})
		}
	}
}
