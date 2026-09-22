package browse_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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

	v := venvtest.NewOSWithEmptyEnv()
	v.Terminal = &venv.Terminal{
		StdinIsTTY:  func() bool { return true },
		StdoutIsTTY: func() bool { return true },
		StderrIsTTY: func() bool { return true },
		Width:       func() int { return 0 },
	}

	done := make(chan error, 1)

	go func() {
		done <- browse.Run(ctx, logger.CreateLogger(), v, browse.NewOptions(opts))
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		require.Fail(t, "browse.Run did not return; the discovery goroutine likely never unwound")
	}
}

// TestRunWithoutTerminalFails covers a run with no terminal to draw on, such
// as a CI job: the browser has no other output, so it reports that instead of
// starting.
func TestRunWithoutTerminalFails(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(t.TempDir(), "terragrunt.hcl"))
	require.NoError(t, err)

	err = browse.Run(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), browse.NewOptions(opts))
	require.ErrorIs(t, err, viewtui.ErrNoTerminal)
}

func TestNewCommandIsNamedBrowse(t *testing.T) {
	t.Parallel()

	cmd := browse.NewCommand(
		logger.CreateLogger(),
		options.NewTerragruntOptions(vexec.NewOSExec()),
		venvtest.NewOSWithEmptyEnv(),
	)

	require.NotNil(t, cmd)
	assert.Equal(t, browse.CommandName, cmd.Name)
	assert.NotEmpty(t, cmd.Flags)
}
