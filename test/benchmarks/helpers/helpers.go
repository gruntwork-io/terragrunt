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

func GenerateNUnits(b *testing.B, tmpDir string, n int, tgConfig string, tfConfig string) {
	b.Helper()

	for i := range n {
		unitDir := filepath.Join(tmpDir, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, 0755))

		// Create an empty `terragrunt.hcl` file
		unitTerragruntConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
		require.NoError(b, os.WriteFile(unitTerragruntConfigPath, []byte(tgConfig), 0644))

		// Create an empty `main.tf` file
		unitMainTfPath := filepath.Join(unitDir, "main.tf")
		require.NoError(b, os.WriteFile(unitMainTfPath, []byte(tfConfig), 0644))
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
