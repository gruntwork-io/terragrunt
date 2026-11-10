package print_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// invalidHCL fails config parsing before any source is downloaded.
const invalidHCL = "this is ) not ( valid hcl"

// missingSourceHCL points at a module directory that does not exist, so the source download fails.
const missingSourceHCL = `
terraform {
  source = "../modules/missing"
}
`

var (
	memRoot = venvtest.Root("/print")

	errWrite = errors.New("write refused")
)

// TestNewCommandExposesThePrintFlags pins the flags users pass to `info print`.
func TestNewCommandExposesThePrintFlags(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	cmd := print.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v, new(rewrite.Store))

	assert.Equal(t, print.CommandName, cmd.Name)

	for _, name := range []string{shared.AllFlagName, shared.ConfigFlagName, shared.DownloadDirFlagName} {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandPrintsTheUnitContext pins that the unit's context is printed even when preparing it fails.
func TestNewCommandPrintsTheUnitContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		hcl          string
		wantCacheDir bool
	}{
		{name: "valid config runs from the download cache", wantCacheDir: true},
		{name: "invalid config", hcl: invalidHCL},
		{name: "source that fails to download", hcl: missingSourceHCL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := new(bytes.Buffer)
			v := venv.MemVenv().WithWriter(out).WithFS(venvtest.NewFS(t, memRoot, map[string]string{
				"unit/terragrunt.hcl": tc.hcl,
			}))

			configPath := filepath.Join(memRoot, "unit", "terragrunt.hcl")
			opts := newOpts(t, v, configPath)
			opts.TFPath = "tofu"

			require.NoError(t, runCommand(t, print.NewCommand(logger.CreateLogger(), opts, v, new(rewrite.Store))))

			var got print.InfoOutput

			require.NoError(t, json.Unmarshal(out.Bytes(), &got))
			assert.Equal(t, configPath, got.ConfigPath)
			assert.Equal(t, tc.wantCacheDir, isCacheDir(filepath.Join(memRoot, "unit"), got.WorkingDir), got.WorkingDir)
			assert.Equal(t, "tofu", got.TerraformBinary)
			assert.Contains(t, out.String(), "\n  \"config_path\"", "a single unit is printed indented")
		})
	}
}

// TestNewCommandAllPrintsOneLinePerUnit pins that --all prints each unit's context as one JSON line.
func TestNewCommandAllPrintsOneLinePerUnit(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	v := venv.MemVenv().WithWriter(out).WithFS(venvtest.NewFS(t, memRoot, map[string]string{
		"a/terragrunt.hcl": "",
		"b/terragrunt.hcl": invalidHCL,
	}))

	require.NoError(
		t,
		runCommand(t, print.NewCommand(logger.CreateLogger(), newOpts(t, v, ""), v, new(rewrite.Store)), "--all"),
	)

	var workingDirs []string

	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		var got print.InfoOutput

		require.NoError(t, json.Unmarshal(scanner.Bytes(), &got), "each line is one JSON document")

		workingDirs = append(workingDirs, got.WorkingDir)
	}

	require.NoError(t, scanner.Err())
	require.Len(t, workingDirs, 2)
	assert.True(
		t,
		isCacheDir(filepath.Join(memRoot, "a"), workingDirs[0]),
		"a prepared into its cache: %s", workingDirs[0],
	)
	assert.Equal(t, filepath.Join(memRoot, "b"), workingDirs[1], "b failed to parse and keeps its own directory")
}

// TestRunReportsAWriteFailure pins how a failed write reaches the caller in single and --all runs.
func TestRunReportsAWriteFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configPath string
		runAll     bool
		failFast   bool
		wantJoined bool
	}{
		{name: "single unit", configPath: filepath.Join(memRoot, "a", "terragrunt.hcl")},
		{name: "all units are joined", runAll: true, wantJoined: true},
		{name: "fail fast returns the first failure", runAll: true, failFast: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithWriter(failingWriter{}).WithFS(venvtest.NewFS(t, memRoot, map[string]string{
				"a/terragrunt.hcl": "",
				"b/terragrunt.hcl": "",
			}))

			opts := newOpts(t, v, tc.configPath)
			opts.RunAll = tc.runAll
			opts.FailFast = tc.failFast

			err := print.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)
			require.ErrorIs(t, err, errWrite)

			var joined interface{ Unwrap() []error }

			assert.Equal(t, tc.wantJoined, errors.As(err, &joined), "only --all without fail fast joins errors")
		})
	}
}

// TestRunIgnoresAWriteFailureAfterAPreparationFailure pins that preparation failures stay non-fatal.
func TestRunIgnoresAWriteFailureAfterAPreparationFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hcl  string
	}{
		{name: "invalid config", hcl: invalidHCL},
		{name: "source that fails to download", hcl: missingSourceHCL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithWriter(failingWriter{}).WithFS(venvtest.NewFS(t, memRoot, map[string]string{
				"unit/terragrunt.hcl": tc.hcl,
			}))

			opts := newOpts(t, v, filepath.Join(memRoot, "unit", "terragrunt.hcl"))

			require.NoError(t, print.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts))
		})
	}
}

// TestRunAllFailsOnAMissingWorkingDir pins that --all surfaces the discovery failure.
func TestRunAllFailsOnAMissingWorkingDir(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	opts := newOpts(t, v, "")
	opts.WorkingDir = filepath.Join(memRoot, "missing")
	opts.RunAll = true

	err := print.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// isCacheDir reports whether dir sits inside unitDir's download cache.
func isCacheDir(unitDir, dir string) bool {
	return strings.HasPrefix(dir, filepath.Join(unitDir, ".terragrunt-cache")+string(filepath.Separator))
}

// failingWriter refuses every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}

// newOpts returns options rooted at the in-memory fixture, reading configPath when set.
func newOpts(t *testing.T, v *venv.Venv, configPath string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, configPath)
	require.NoError(t, err)

	if configPath == "" {
		opts.WorkingDir = memRoot
	}

	opts.RootWorkingDir = memRoot
	opts.NonInteractive = true

	return opts
}

// runCommand runs cmd with args the way the app does and returns its error.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = io.Discard
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
