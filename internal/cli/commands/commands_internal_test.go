package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/urfave/cli/v2"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialSetupResolvesWorkingDirSymlinks(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	tmpDir, err := os.MkdirTemp("", "resolve-working-dir-symlink-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("failed to remove temp dir %s: %v", tmpDir, err)
		}
	})

	source := filepath.Join(tmpDir, "source")
	require.NoError(t, os.Mkdir(source, 0o755))

	link := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(source, link))

	expectedDir, err := filepath.EvalSymlinks(link)
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = link // unresolved symlink path

	cliApp := &clihelper.App{App: &cli.App{Version: "0.0.0"}}
	appCtx := clihelper.NewAppContext(cliApp, clihelper.Args{})
	cmdCtx := appCtx.NewCommandContext(&clihelper.Command{Name: runcmd.CommandName}, clihelper.Args{})

	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	l := log.New(log.WithFormatter(formatter))
	require.NoError(t, initialSetup(cmdCtx, l, opts))

	assert.Equal(t, expectedDir, opts.WorkingDir,
		"WorkingDir should be resolved to the symlink target, not the symlink path itself")
}
