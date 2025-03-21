package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

func RunTerragruntCommand(tb *testing.B, args ...string) {
	writer := io.Discard
	errwriter := io.Discard

	opts := options.NewTerragruntOptionsWithWriters(writer, errwriter)
	app := cli.NewApp(opts) //nolint:contextcheck

	ctx := log.ContextWithLogger(context.Background(), opts.Logger)

	err := app.RunContext(ctx, args)
	require.NoError(tb, err)
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
