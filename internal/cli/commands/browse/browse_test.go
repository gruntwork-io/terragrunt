package browse_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunUnwindsCleanlyWhenContextCancelledWithRacing drives the whole browse
// command headlessly. A context cancelled before the browser's loop starts
// makes the interactive program exit at once, and Run must then cancel the
// background discovery, wait for its goroutine to unwind, and return without a
// deadlock or a leaked worker. Run under -race, it also guards the discovery
// goroutine's handoff to the browser against data races.
func TestRunUnwindsCleanlyWhenContextCancelledWithRacing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	fs := vfs.NewOSFS()
	require.NoError(t, fs.MkdirAll(filepath.Join(dir, "vpc"), 0o755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(dir, "vpc", "terragrunt.hcl"), []byte("\n"), 0o644))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(dir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = dir

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan error, 1)

	go func() {
		done <- browse.Run(ctx, logger.CreateLogger(), venv.OSVenv(), browse.NewOptions(opts))
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("browse.Run did not return; the discovery goroutine likely never unwound")
	}
}

func TestNewCommandIsNamedBrowse(t *testing.T) {
	t.Parallel()

	cmd := browse.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(), venv.OSVenv())

	require.NotNil(t, cmd)
	assert.Equal(t, browse.CommandName, cmd.Name)
	assert.NotEmpty(t, cmd.Flags)
}
