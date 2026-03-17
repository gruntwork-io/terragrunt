package test_test

import (
	"io"
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

	b.Run("init with auto provider cache dir", func(b *testing.B) {
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
			name: "with auto provider cache dir",
			args: []string{},
		},
		{
			name: "with provider cache server",
			args: []string{"--provider-cache"},
		},
	}

	for _, count := range counts {
		for _, cacheType := range cacheTypes {
			name := strconv.Itoa(count) + " units " + cacheType.name

			b.Run(name, func(b *testing.B) {
				tmpDir := b.TempDir()

				setup(tmpDir, count)

				args := make([]string, 0, 8+len(cacheType.args))
				args = append(args,
					"terragrunt",
					"run",
					"--all",
					"init",
					"--source-update",
					"--non-interactive",
					"--working-dir",
					tmpDir,
				)

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

// BenchmarkAutoProviderCacheDirRegistryHashes benchmarks Terragrunt init with the new OpenTofu registry hashes used
func BenchmarkAutoProviderCacheDirRegistryHashes(b *testing.B) {
	setup := func(tmpDir string) {
		fixtureSource := filepath.Join("..", "fixtures", "auto-provider-cache-dir", "heavy", "unit")

		srcTerragruntCfg, err := os.Open(filepath.Join(fixtureSource, "terragrunt.hcl"))
		require.NoError(b, err)

		srcMainTF, err := os.Open(filepath.Join(fixtureSource, "main.tf"))
		require.NoError(b, err)

		terragruntConfigPath := filepath.Join(tmpDir, "terragrunt.hcl")

		terragruntCfg, err := os.OpenFile(
			terragruntConfigPath,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC,
			helpers.DefaultFilePermissions,
		)
		require.NoError(b, err)

		mainTfPath := filepath.Join(tmpDir, "main.tf")

		mainTf, err := os.OpenFile(
			mainTfPath,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC,
			helpers.DefaultFilePermissions,
		)
		require.NoError(b, err)

		_, err = io.Copy(terragruntCfg, srcTerragruntCfg)
		require.NoError(b, err)

		_, err = io.Copy(mainTf, srcMainTF)
		require.NoError(b, err)

		err = mainTf.Close()
		require.NoError(b, err)

		err = terragruntCfg.Close()
		require.NoError(b, err)

		err = srcMainTF.Close()
		require.NoError(b, err)

		err = srcTerragruntCfg.Close()
		require.NoError(b, err)

		helpers.RunTerragruntCommand(
			b,
			"terragrunt",
			"init",
			"--non-interactive",
			"--working-dir",
			tmpDir,
		)
	}

	latestTofuPath := os.Getenv("LATEST_TOFU_PATH")
	require.NotEmpty(b, latestTofuPath)
	require.FileExists(b, latestTofuPath)

	nightlyTofuPath := os.Getenv("NIGHTLY_TOFU_PATH")
	require.NotEmpty(b, nightlyTofuPath)
	require.FileExists(b, nightlyTofuPath)

	b.Run("latest init", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		for b.Loop() {
			err := os.RemoveAll(filepath.Join(tmpDir, ".terraform.lock.hcl"))
			require.NoError(b, err)

			err = os.RemoveAll(filepath.Join(tmpDir, ".terragrunt-cache"))
			require.NoError(b, err)

			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--tf-path",
				latestTofuPath,
				"--non-interactive",
				"--working-dir",
				tmpDir,
			)
		}
	})

	b.Run("nightly init", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		for b.Loop() {
			err := os.Remove(filepath.Join(tmpDir, ".terraform.lock.hcl"))
			require.NoError(b, err)

			err = os.RemoveAll(filepath.Join(tmpDir, ".terragrunt-cache"))
			require.NoError(b, err)

			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--tf-path",
				nightlyTofuPath,
				"--non-interactive",
				"--working-dir",
				tmpDir,
			)
		}
	})

	b.Run("latest init w lockfile", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		for b.Loop() {
			err := os.RemoveAll(filepath.Join(tmpDir, ".terragrunt-cache"))
			require.NoError(b, err)

			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--tf-path",
				latestTofuPath,
				"--non-interactive",
				"--working-dir",
				tmpDir,
			)
		}
	})

	b.Run("nightly init w lockfile", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		for b.Loop() {
			err := os.RemoveAll(filepath.Join(tmpDir, ".terragrunt-cache"))
			require.NoError(b, err)

			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--tf-path",
				nightlyTofuPath,
				"--non-interactive",
				"--working-dir",
				tmpDir,
			)
		}
	})
}
