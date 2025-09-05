package test_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

// BenchmarkCASInit benchmarks Terragrunt init with remote source with and without CAS enabled
func BenchmarkCASInit(b *testing.B) {
	setup := func(tmpDir string) {
		// Copy the remote fixture content to our test directory
		remoteFixtureSource := filepath.Join("..", "fixtures", "download", "remote")
		remoteTerragruntConfigPath := filepath.Join(tmpDir, "terragrunt.hcl")

		// Read the original terragrunt.hcl from the remote fixture
		originalConfig, err := os.ReadFile(filepath.Join(remoteFixtureSource, "terragrunt.hcl"))
		require.NoError(b, err)

		// Write the config to our test directory
		require.NoError(b, os.WriteFile(remoteTerragruntConfigPath, originalConfig, helpers.DefaultFilePermissions))

		// Run initial init to avoid noise from the first iteration being slower
		helpers.RunTerragruntCommand(
			b,
			"terragrunt",
			"init",
			"--experiment", "cas",
			"--non-interactive",
			"--provider-cache",
			"--working-dir",
			tmpDir,
		)
	}

	b.Run("remote init without CAS", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		b.ResetTimer()

		for b.Loop() {
			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--non-interactive",
				"--provider-cache",
				"--source-update",
				"--working-dir", tmpDir)
		}

		b.StopTimer()
	})

	b.Run("remote init with CAS", func(b *testing.B) {
		tmpDir := b.TempDir()

		setup(tmpDir)

		b.ResetTimer()

		for b.Loop() {
			helpers.RunTerragruntCommand(
				b,
				"terragrunt",
				"init",
				"--experiment", "cas",
				"--non-interactive",
				"--provider-cache",
				"--source-update",
				"--working-dir",
				tmpDir)
		}

		b.StopTimer()
	})
}

// BenchmarkCASWithManyUnits benchmarks Terragrunt init with many remote units with and without CAS enabled
func BenchmarkCASWithManyUnits(b *testing.B) {
	setup := func(tmpDir string, count int) {
		remoteFixtureSource := filepath.Join("..", "fixtures", "download", "remote")
		originalConfig, err := os.ReadFile(filepath.Join(remoteFixtureSource, "terragrunt.hcl"))
		require.NoError(b, err)

		// Generate units with the remote configuration
		for i := range count {
			unitDir := filepath.Join(tmpDir, "unit-"+strconv.Itoa(i))
			require.NoError(b, os.MkdirAll(unitDir, helpers.DefaultDirPermissions))

			unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
			require.NoError(b, os.WriteFile(unitTerragruntConfigPath, originalConfig, helpers.DefaultFilePermissions))
		}

		// Run initial init to avoid noise from the first iteration being slower
		helpers.RunTerragruntCommand(
			b,
			"terragrunt",
			"run",
			"--all",
			"init",
			"--non-interactive",
			"--provider-cache",
			"--source-update",
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
		32,
		64,
		128,
	}

	for _, count := range counts {
		for _, cas := range []bool{false, true} {
			name := strconv.Itoa(count) + " remote units " + (func() string {
				if cas {
					return "with CAS"
				}
				return "without CAS"
			})()

			b.Run(name, func(b *testing.B) {
				tmpDir := b.TempDir()

				setup(tmpDir, count)

				args := []string{
					"terragrunt",
					"run",
					"--all",
					"init",
					"--non-interactive",
					"--provider-cache",
					"--source-update",
					"--working-dir",
					tmpDir,
				}

				if cas {
					args = append(args, "--experiment", "cas")
				}

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
