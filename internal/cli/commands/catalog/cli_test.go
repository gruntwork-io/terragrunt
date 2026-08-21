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
	"github.com/gruntwork-io/terragrunt/internal/experiment"
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
		logger.CreateLogger(), options.NewTerragruntOptions(), venvtest.New(),
	)

	assert.Equal(t, catalog.CommandName, cmd.Name)
	assert.NotNil(t, cmd.Flags.Get(catalog.FormatFlagName))
	assert.NotNil(t, cmd.Flags.Get(catalog.IgnoreFileFlagName))
}

// TestNewCommandBeforeGatesNonInteractiveFormats pins the experiment gate on
// the non-interactive formats: their output is a compatibility promise, so it
// stays behind the experiment until the shape of it is settled.
func TestNewCommandBeforeGatesNonInteractiveFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr        error
		name           string
		format         string
		withExperiment bool
		wantInvalid    bool
	}{
		{name: "tui needs no experiment", format: catalog.FormatTUI},
		{
			name:    "jsonl without the experiment",
			format:  catalog.FormatJSONL,
			wantErr: catalog.ErrFormatRequiresExperiment,
		},
		{
			name:    "md without the experiment",
			format:  catalog.FormatMD,
			wantErr: catalog.ErrFormatRequiresExperiment,
		},
		{name: "jsonl with the experiment", format: catalog.FormatJSONL, withExperiment: true},
		{name: "md with the experiment", format: catalog.FormatMD, withExperiment: true},
		{name: "unknown format", format: "yaml", wantInvalid: true},
		{
			name:           "unknown format with the experiment",
			format:         "yaml",
			withExperiment: true,
			wantInvalid:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			if tc.withExperiment {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.CatalogFormat))
			}

			cmd := catalog.NewCommand(logger.CreateLogger(), opts, venvtest.New())
			require.NoError(
				t,
				cmd.Flags.Parse(clihelper.Args{"--" + catalog.FormatFlagName, tc.format}, map[string]string{}),
			)

			err := cmd.Before(t.Context(), &clihelper.Context{})

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantInvalid:
				require.Error(t, err)
				require.NotErrorIs(t, err, catalog.ErrFormatRequiresExperiment,
					"an unusable format must fail validation rather than the experiment gate")
				assertGeneralError(t, err)
			default:
				require.NoError(t, err)
			}
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

	opts := options.NewTerragruntOptions()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.CatalogFormat))

	cmd := catalog.NewCommand(logger.CreateLogger(), opts, v)

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
			require.NoError(t, os.WriteFile(filepath.Join(dir, ignoreFileName), []byte("vendor\n"), 0o644))

			opts := catalog.NewOptions(options.NewTerragruntOptions())
			opts.RootWorkingDir = dir

			if !tc.noWorkingDir {
				opts.WorkingDir = dir
			}

			flags := catalog.NewFlags(opts, nil)
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
