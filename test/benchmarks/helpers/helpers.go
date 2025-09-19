// Package helpers provides helper functions for the integration benchmarks.
package helpers

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

const (
	// DefaultDirPermissions specifies the default file mode for creating directories.
	// rwxr-xr-x (owner can read, write, execute; group and others can read, execute)
	DefaultDirPermissions = 0755
	// DefaultFilePermissions specifies the default file mode for creating files.
	// rw-r--r-- (owner can read, write; group and others can read)
	DefaultFilePermissions = 0644
)

// RunTerragruntCommand runs a Terragrunt command and logs the output to io.Discard.
func RunTerragruntCommand(b *testing.B, args ...string) {
	b.Helper()

	writer := io.Discard
	errwriter := io.Discard

	opts := options.NewTerragruntOptionsWithWriters(writer, errwriter)

	l := logger.CreateLogger().WithOptions(log.WithOutput(io.Discard))

	app := cli.NewApp(l, opts) //nolint:contextcheck

	ctx := log.ContextWithLogger(b.Context(), l)

	err := app.RunContext(ctx, args)
	require.NoError(b, err)
}

// GenerateNUnits generates n units in the given temporary directory.
func GenerateNUnits(b *testing.B, dir string, n int, tgConfig string, tfConfig string) {
	b.Helper()

	for i := range n {
		unitDir := filepath.Join(dir, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, DefaultDirPermissions))

		// Create an empty `terragrunt.hcl` file
		unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(unitTerragruntConfigPath, []byte(tgConfig), DefaultFilePermissions))

		// Create an empty `main.tf` file
		unitMainTfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(unitMainTfPath, []byte(tfConfig), DefaultFilePermissions))
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
	require.NoError(b, os.WriteFile(rootTerragruntConfigPath, []byte(emptyRootConfig), DefaultFilePermissions))

	// Generate n units
	GenerateNUnits(b, dir, n, includeRootConfig, emptyMainTf)
}

func Init(b *testing.B, dir string) {
	b.Helper()

	// Measure plan time
	planStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run", "--all", "init", "--non-interactive", "--working-dir", dir)

	planDuration := time.Since(planStart)

	b.ReportMetric(float64(planDuration.Seconds()), "init_s/op")
}

func Plan(b *testing.B, dir string) {
	b.Helper()

	// Measure plan time
	planStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run", "--all", "plan", "--non-interactive", "--working-dir", dir)

	planDuration := time.Since(planStart)

	b.ReportMetric(float64(planDuration.Seconds()), "plan_s/op")
}

func Apply(b *testing.B, dir string) {
	b.Helper()

	// Track apply time
	applyStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run", "--all", "apply", "--non-interactive", "--working-dir", dir)

	applyDuration := time.Since(applyStart)

	b.ReportMetric(float64(applyDuration.Seconds()), "apply_s/op")
}

func ApplyWithRunnerPool(b *testing.B, dir string) {
	b.Helper()

	// Track apply time
	applyStart := time.Now()

	RunTerragruntCommand(b, "terragrunt", "run", "--all", "apply", "--non-interactive", "--experiment", "runner-pool", "--working-dir", dir)

	applyDuration := time.Since(applyStart)

	b.ReportMetric(float64(applyDuration.Seconds()), "apply_s/op")
}
