// Package helpers provides helper functions for the integration benchmarks.
package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

// RunTerragruntCommand runs a Terragrunt command and logs the output to io.Discard.
func RunTerragruntCommand(b *testing.B, args ...string) {
	b.Helper()

	writer := io.Discard
	errwriter := io.Discard

	opts := options.NewTerragruntOptionsWithWriters(writer, errwriter)
	app := cli.NewApp(opts) //nolint:contextcheck

	ctx := log.ContextWithLogger(context.Background(), opts.Logger)

	err := app.RunContext(ctx, args)
	require.NoError(b, err)
}

// GenerateNUnits generates n units in the given temporary directory.
func GenerateNUnits(b *testing.B, dir string, n int, tgConfig string, tfConfig string) {
	b.Helper()

	for i := range n {
		unitDir := filepath.Join(dir, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, 0755))

		// Create an empty `terragrunt.hcl` file
		unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(unitTerragruntConfigPath, []byte(tgConfig), 0644))

		// Create an empty `main.tf` file
		unitMainTfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(unitMainTfPath, []byte(tfConfig), 0644))
	}
}

// GenerateEmptyUnits generates n empty units in the given temporary directory.
func GenerateEmptyUnits(b *testing.B, dir string, n int) {
	b.Helper()

	emptyRootConfig := ``
	includeRootConfig := `include "root" {
		path = find_in_parent_folders("root.hcl")
}
`
	emptyMainTf := ``

	rootTerragruntConfigPath := filepath.Join(dir, "root.hcl")

	// Create an empty `root.hcl` file
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), 0644))

	// Generate n units
	GenerateNUnits(b, dir, n, includeRootConfig, emptyMainTf)
}

// GenerateDependencyTrain generates a dependency train in the given temporary directory.
// A dependency train is a set of units where each unit depends on the previous unit (except for the first unit).
func GenerateDependencyTrain(b *testing.B, dir string, n int) {
	b.Helper()

	if n <= 1 {
		b.Fatalf("n must be greater than 1")
	}

	// Create the root config
	rootConfig := ``

	rootConfigPath := filepath.Join(dir, "root.hcl")
	require.NoError(b, os.WriteFile(rootConfigPath, []byte(rootConfig), 0644))

	// Create the first unit
	unitDir := filepath.Join(dir, "unit-0")
	require.NoError(b, os.MkdirAll(unitDir, 0755))

	// Create an empty `terragrunt.hcl` file
	unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
	require.NoError(b, os.WriteFile(unitTerragruntConfigPath, []byte(`include "root" {
	path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

inputs = {
	unit_count = `+strconv.Itoa(n)+`
}
`), 0644))

	// Create an empty `main.tf` file
	unitMainTfPath := filepath.Join(unitDir, "main.tf")
	require.NoError(b, os.WriteFile(unitMainTfPath, []byte(`variable "unit_count" {
	type = number
}

resource "null_resource" "this" {
	triggers = {
		always_run = timestamp()
	}
}

output "unit_count" {
	value = var.unit_count
}
`), 0644))

	// Create N-1 units
	for i := 1; i < n; i++ {
		unitDir := filepath.Join(dir, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, 0755))

		// Create an empty `terragrunt.hcl` file
		unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(unitTerragruntConfigPath, []byte(`include "root" {
	path = find_in_parent_folders("root.hcl")
}

terraform {
	source = "."
}

dependency "unit_`+strconv.Itoa(i-1)+`" {
	config_path = "../unit-`+strconv.Itoa(i-1)+`"

	mock_outputs = {
		unit_count = 0
	}

	mock_outputs_allowed_terraform_commands = ["plan"]
}

inputs = {
	unit_count = dependency.unit_`+strconv.Itoa(i-1)+`.outputs.unit_count + 1
}

`), 0644))

		// Create an empty `main.tf` file
		unitMainTfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(unitMainTfPath, []byte(`variable "unit_count" {
	type = number
}

output "unit_count" {
	value = var.unit_count
}
		`), 0644))
	}
}

func PlanApplyDestroy(b *testing.B, dir string) {
	b.Helper()

	// Measure plan time
	planStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run-all", "plan", "--non-interactive", "--working-dir", dir)

	planDuration := time.Since(planStart)

	// Measure apply time
	applyStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run-all", "apply", "-auto-approve", "--non-interactive", "--working-dir", dir)

	applyDuration := time.Since(applyStart)

	// Measure destroy time
	destroyStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run-all", "destroy", "-auto-approve", "--non-interactive", "--working-dir", dir)

	destroyDuration := time.Since(destroyStart)

	b.ReportMetric(float64(planDuration.Seconds()), "plan_s/op")
	b.ReportMetric(float64(applyDuration.Seconds()), "apply_s/op")
	b.ReportMetric(float64(destroyDuration.Seconds()), "destroy_s/op")
}
