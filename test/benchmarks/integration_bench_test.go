package test_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
	"github.com/stretchr/testify/require"
)

func BenchmarkEmptyTerragruntPlanApply(b *testing.B) {

	emptyTerragruntConfig := ``
	emptyMainTf := ``

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}
`

	b.Run("1 unit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create a temporary directory for each test
			tmpDir := b.TempDir()
			terragruntConfigPath := filepath.Join(tmpDir, "terragrunt.hcl")

			// Create an empty `terragrunt.hcl` file
			require.NoError(b, os.WriteFile(terragruntConfigPath, []byte(emptyTerragruntConfig), 0644))

			// Create an empty `main.tf` file
			mainTfPath := filepath.Join(tmpDir, "main.tf")
			require.NoError(b, os.WriteFile(mainTfPath, []byte(emptyMainTf), 0644))
			b.StartTimer()

			// Measure plan time
			planStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "plan", "--non-interactive", "--working-dir", tmpDir)
			planDuration := time.Since(planStart)

			// Measure apply time
			applyStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "apply", "-auto-approve", "--non-interactive", "--working-dir", tmpDir)
			applyDuration := time.Since(applyStart)

			b.ReportMetric(float64(planDuration.Milliseconds()), "plan_ms/op")
			b.ReportMetric(float64(applyDuration.Milliseconds()), "apply_ms/op")
		}
	})

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

			// Measure plan time
			planStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "plan", "--non-interactive", "--working-dir", tmpDir)
			planDuration := time.Since(planStart)

			// Measure apply time
			applyStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "apply", "-auto-approve", "--non-interactive", "--working-dir", tmpDir)
			applyDuration := time.Since(applyStart)

			b.ReportMetric(float64(planDuration.Milliseconds()), "plan_ms/op")
			b.ReportMetric(float64(applyDuration.Milliseconds()), "apply_ms/op")
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

			// Measure plan time
			planStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "plan", "--non-interactive", "--working-dir", tmpDir)
			planDuration := time.Since(planStart)

			// Measure apply time
			applyStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "apply", "-auto-approve", "--non-interactive", "--working-dir", tmpDir)
			applyDuration := time.Since(applyStart)

			b.ReportMetric(float64(planDuration.Milliseconds()), "plan_ms/op")
			b.ReportMetric(float64(applyDuration.Milliseconds()), "apply_ms/op")
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

			// Measure plan time
			planStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "plan", "--non-interactive", "--working-dir", tmpDir)
			planDuration := time.Since(planStart)

			// Measure apply time
			applyStart := time.Now()
			helpers.RunTerragruntCommand(b, "terragrunt", "run-all", "apply", "-auto-approve", "--non-interactive", "--working-dir", tmpDir)
			applyDuration := time.Since(applyStart)

			b.ReportMetric(float64(planDuration.Milliseconds()), "plan_ms/op")
			b.ReportMetric(float64(applyDuration.Milliseconds()), "apply_ms/op")
		}
	})
}
