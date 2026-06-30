package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

// BenchmarkStackOutputParallelism measures the wall-clock cost of `terragrunt stack output`
// against a synthetic stack of identical units, sweeping --parallelism from serial (1) up to
// 8x the number of CPU cores. The intent is to surface whether allowing more concurrent
// `tofu output` child processes than cores keeps paying off (I/O bound) or starts to regress
// (process-spawn, file-descriptor, or scheduler contention).
//
// Run with a longer benchtime to get stable numbers; one iteration shells out to tofu N times:
//
//	go test -bench BenchmarkStackOutputParallelism -benchtime 3x -timeout 30m ./test/benchmarks/
func BenchmarkStackOutputParallelism(b *testing.B) {
	const unitCount = 50

	tmpDir := b.TempDir()
	livePath := filepath.Join(tmpDir, "live")
	appPath := filepath.Join(tmpDir, "units", "app")

	require.NoError(b, os.MkdirAll(livePath, helpers.DefaultDirPermissions))
	require.NoError(b, os.MkdirAll(appPath, helpers.DefaultDirPermissions))

	require.NoError(b, os.WriteFile(
		filepath.Join(appPath, "main.tf"),
		[]byte(`output "value" {
  value = "ok"
}
`),
		helpers.DefaultFilePermissions,
	))
	require.NoError(b, os.WriteFile(
		filepath.Join(appPath, "terragrunt.hcl"),
		[]byte(`terraform {
  source = "."
}
`),
		helpers.DefaultFilePermissions,
	))

	var stackHCL strings.Builder
	for i := range unitCount {
		fmt.Fprintf(&stackHCL, `unit "u%03d" {
  source = "../units/app"
  path   = "u%03d"
}

`, i, i)
	}

	require.NoError(b, os.WriteFile(
		filepath.Join(livePath, "terragrunt.stack.hcl"),
		[]byte(stackHCL.String()),
		helpers.DefaultFilePermissions,
	))

	helpers.RunTerragruntCommand(b, "terragrunt", "stack", "run", "apply", "--non-interactive", "--working-dir", livePath)

	cpus := runtime.NumCPU()
	levels := []int{1, max(cpus/2, 1), cpus, 2 * cpus, 4 * cpus, 8 * cpus}
	seen := make(map[int]struct{}, len(levels))

	for _, level := range levels {
		if _, dup := seen[level]; dup {
			continue
		}

		seen[level] = struct{}{}

		b.Run("parallelism="+strconv.Itoa(level), func(b *testing.B) {
			b.ResetTimer()

			for b.Loop() {
				helpers.RunTerragruntCommand(
					b,
					"terragrunt", "stack", "output",
					"--parallelism", strconv.Itoa(level),
					"--non-interactive",
					"--working-dir", livePath,
				)
			}

			b.StopTimer()
		})
	}
}
