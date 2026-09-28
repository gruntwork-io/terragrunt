package catalog_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// ignoreFileName is the fixture written for the ignore-file flag cases.
const ignoreFileName = ".terragrunt-catalog-ignore"

// TestNewCommandExposesTheCatalogFlags pins the command name and the flags
// users invoke it with, both of which are part of the CLI contract.
func TestNewCommandExposesTheCatalogFlags(t *testing.T) {
	t.Parallel()

	cmd := catalog.NewCommand(
		logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), venvtest.New(),
	)

	assert.Equal(t, catalog.CommandName, cmd.Name)
	assert.NotNil(t, cmd.Flags.Get(catalog.FormatFlagName))
	assert.NotNil(t, cmd.Flags.Get(catalog.IgnoreFileFlagName))
}

// TestNewCommandBeforeValidatesTheFormat pins which formats the command
// accepts with no experiment enabled. An unknown format exits with the general
// error status.
func TestNewCommandBeforeValidatesTheFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		format      string
		wantInvalid bool
	}{
		{name: "tui", format: catalog.FormatTUI},
		{name: "jsonl", format: catalog.FormatJSONL},
		{name: "md", format: catalog.FormatMD},
		{name: "unknown format", format: "yaml", wantInvalid: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := catalog.NewCommand(
				logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), venvtest.New(),
			)
			require.NoError(
				t,
				cmd.Flags.Parse(
					clihelper.Args{"--" + catalog.FormatFlagName, tc.format},
					map[string]string{},
				),
			)

			err := cmd.Before(t.Context(), &clihelper.Context{})

			if tc.wantInvalid {
				assertGeneralError(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestNewCommandActionLoadsThePositionalSource pins that the first argument of
// `terragrunt catalog <source>` is the source that gets browsed.
func TestNewCommandActionLoadsThePositionalSource(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	v := venvtest.New().WithWriter(&buf)
	repoDir := "/catalog-positional/repo"

	writeLocalRepo(t, v, repoDir)

	cmd := catalog.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), v)

	require.NoError(t, cmd.Flags.Parse(
		clihelper.Args{"--" + catalog.FormatFlagName, catalog.FormatJSONL},
		map[string]string{},
	))
	require.NoError(t, cmd.Before(t.Context(), &clihelper.Context{}))
	require.NoError(t, cmd.Action(
		t.Context(), clihelper.NewAppContext(nil, clihelper.Args{repoDir}),
	))

	assert.Equal(t, []string{"alpha", "bravo"}, sortedDirs(t, buf.String()))
}

// TestNewCommandDefaultsToJSONLWithoutATerminal pins that a run with no
// terminal and no --format writes JSON Lines, so piping the command needs no
// flag.
func TestNewCommandDefaultsToJSONLWithoutATerminal(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	v := venvtest.New().WithWriter(&buf)
	repoDir := "/catalog-default-format/repo"

	writeLocalRepo(t, v, repoDir)

	cmd := catalog.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), v)

	require.NoError(t, cmd.Flags.Parse(clihelper.Args{}, map[string]string{}))
	require.NoError(t, cmd.Before(t.Context(), &clihelper.Context{}))
	require.NoError(t, cmd.Action(
		t.Context(), clihelper.NewAppContext(nil, clihelper.Args{repoDir}),
	))

	assert.Equal(t, []string{"alpha", "bravo"}, sortedDirs(t, buf.String()))
}

// TestNewFlagsIgnoreFileAction pins how the ignore-file path is resolved and
// rejected: the flag names a file the user expects to be read, so a path that
// cannot be read has to fail the run rather than be silently ignored.
func TestNewFlagsIgnoreFileAction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr      error
		value        func(dir string) string
		name         string
		noWorkingDir bool
		wantRejected bool
		wantNoop     bool
	}{
		{
			name:  "absolute path to a file",
			value: func(dir string) string { return filepath.Join(dir, ignoreFileName) },
		},
		{
			name:  "relative path resolved against the working dir",
			value: func(string) string { return ignoreFileName },
		},
		{
			name:         "relative path falling back to the root working dir",
			value:        func(string) string { return ignoreFileName },
			noWorkingDir: true,
		},
		{
			name:     "no value",
			value:    func(string) string { return "" },
			wantNoop: true,
		},
		{
			name:    "missing file",
			value:   func(dir string) string { return filepath.Join(dir, "absent") },
			wantErr: fs.ErrNotExist,
		},
		{
			name:         "directory",
			value:        func(dir string) string { return dir },
			wantRejected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The action stats the path on the real filesystem.
			dir := t.TempDir()
			require.NoError(
				t,
				os.WriteFile(filepath.Join(dir, ignoreFileName), []byte("vendor\n"), 0o644),
			)

			opts := catalog.NewOptions(options.NewTerragruntOptions(vexec.NewOSExec()))
			opts.RootWorkingDir = dir

			if !tc.noWorkingDir {
				opts.WorkingDir = dir
			}

			flags := catalog.NewFlags(vfs.NewOSFS(), opts, nil)
			require.NoError(t, flags.Parse(
				clihelper.Args{"--" + catalog.IgnoreFileFlagName, tc.value(dir)},
				map[string]string{},
			))

			err := flags.RunActions(t.Context(), &clihelper.Context{})

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assertGeneralError(t, err)
			case tc.wantRejected:
				require.Error(t, err)
				require.NotErrorIs(t, err, fs.ErrNotExist,
					"a directory must be rejected on its own, not for failing to resolve")
				assertGeneralError(t, err)
			case tc.wantNoop:
				require.NoError(t, err)
				assert.Empty(t, opts.CatalogIgnoreFile,
					"an empty flag value must not resolve to the working dir")
			default:
				require.NoError(t, err)
				assert.Equal(t, filepath.Join(dir, ignoreFileName), opts.CatalogIgnoreFile,
					"the flag value must reach the run as an absolute path")
			}
		})
	}
}

// assertGeneralError reports that err exits the process with the general
// error status, which is what the shell sees.
func assertGeneralError(t *testing.T, err error) {
	t.Helper()

	var exitErr clihelper.ExitCoder

	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())
}
