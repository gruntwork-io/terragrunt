package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/rcfile"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileFlagDefault(t *testing.T) {
	workingDir := newRCFileDir(t, `{"flags": [{"name": "non-interactive", "default": true}]}`)

	opts := runWithRCFile(t, workingDir, "--experiment", experiment.TerragruntRC, "--version")

	assert.True(t, opts.NonInteractive)
}

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileIsIgnoredWithoutExperiment(t *testing.T) {
	workingDir := newRCFileDir(t, `{"flags": [{"name": "non-interactive", "default": true}]}`)

	opts := runWithRCFile(t, workingDir, "--version")

	assert.False(t, opts.NonInteractive, "the rc file must do nothing while the experiment is off")
}

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileIsOverriddenByArg(t *testing.T) {
	workingDir := newRCFileDir(t, `{"flags": [{"name": "log-show-abs-paths", "default": true}]}`)

	opts := runWithRCFile(
		t,
		workingDir,
		"--experiment", experiment.TerragruntRC,
		"--log-show-abs-paths=false",
		"--version",
	)

	assert.False(t, opts.LogShowAbsPaths, "the command line wins over the rc file")
}

//nolint:paralleltest // isolates the home directory and exports an environment variable.
func TestRCFileEnvSection(t *testing.T) {
	const (
		newVar      = "TG_RCFILE_TEST_NEW"
		existingVar = "TG_RCFILE_TEST_EXISTING"
	)

	t.Setenv(existingVar, "from-shell")
	t.Cleanup(func() { os.Unsetenv(newVar) })

	workingDir := newRCFileDir(t, `{"env": {
	  "`+newVar+`": "from-rc",
	  "`+existingVar+`": "from-rc",
	  "TG_RCFILE_TEST_PATH": "./terraformrc.hcl"
	}}`)

	t.Cleanup(func() { os.Unsetenv("TG_RCFILE_TEST_PATH") })

	runWithRCFile(t, workingDir, "--experiment", experiment.TerragruntRC, "--version")

	assert.Equal(t, "from-rc", os.Getenv(newVar))
	assert.Equal(t, "from-shell", os.Getenv(existingVar), "the shell wins over the rc file")
	assert.Equal(
		t,
		filepath.Join(workingDir, "terraformrc.hcl"),
		os.Getenv("TG_RCFILE_TEST_PATH"),
		"a relative path is resolved against the rc file",
	)
}

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileCommandSection(t *testing.T) {
	workingDir := newRCFileDir(t, `{"commands": [
	  {"name": "hcl fmt", "flags": [{"name": "diff", "default": true}]},
	  {"name": "run", "flags": [{"name": "parallelism", "default": 8}]}
	]}`)

	opts := runWithRCFile(t, workingDir, "--experiment", experiment.TerragruntRC, "hcl", "fmt")

	assert.True(t, opts.Diff, "the entry for the running command is applied")
	assert.NotEqual(t, 8, opts.Parallelism, "the entry for another command is not applied")
}

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileInvalid(t *testing.T) {
	workingDir := newRCFileDir(t, `{"flags": [{"name": "non-interactive"}]}`)

	_, err := runApp(t, workingDir, "--experiment", experiment.TerragruntRC, "--version")
	require.Error(t, err)
	assert.ErrorIs(t, err, rcfile.ErrMissingFlagDefault)
}

//nolint:paralleltest // isolates the home directory so discovery is deterministic.
func TestRCFileInvalidIsIgnoredWithoutExperiment(t *testing.T) {
	workingDir := newRCFileDir(t, `{"flags": [{"name": "non-interactive"}]}`)

	_, err := runApp(t, workingDir, "--version")
	require.NoError(t, err, "a broken rc file must not fail a run that did not ask for the feature")
}

// newRCFileDir returns a working directory holding an rc file with the given contents, and
// points the home directory somewhere empty so that a real rc file cannot be picked up.
func newRCFileDir(t *testing.T, content string) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	workingDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	path := filepath.Join(workingDir, rcfile.BaseName+".json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return workingDir
}

// runWithRCFile runs the app in workingDir and fails the test if the run returns an error.
func runWithRCFile(t *testing.T, workingDir string, args ...string) *options.TerragruntOptions {
	t.Helper()

	opts, err := runApp(t, workingDir, args...)
	require.NoError(t, err)

	return opts
}

// runApp runs the app in workingDir with the given arguments and returns the options it
// ended up with.
func runApp(t *testing.T, workingDir string, args ...string) (*options.TerragruntOptions, error) {
	t.Helper()

	testVenv := venv.OSVenv()
	testVenv.Writers = &writer.Writers{Writer: &bytes.Buffer{}, ErrWriter: &bytes.Buffer{}}

	opts := options.NewTerragruntOptions()
	l := logger.CreateLogger()
	app := cli.NewApp(l, opts, testVenv)

	err := app.Run(l, testVenv, append([]string{"terragrunt", "--working-dir", workingDir}, args...))

	return opts, err
}
