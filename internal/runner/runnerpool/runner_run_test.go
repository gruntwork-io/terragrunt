package runnerpool_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// invalidHCL fails config parsing, so a unit using it errors out in the runner
// task before any Terraform binary is needed.
const invalidHCL = "this is ) not ( valid hcl\n"

func TestRunnerRunExcludedUnitsAreReported(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", "")
	app := newTestUnit(t, tmpDir, "app", "")

	vpc.SetExcluded(true)
	app.SetExcluded(true)

	opts := newRunOpts(t, tmpDir, "plan")
	opts.OutputFolder = filepath.Join(tmpDir, "out")

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(
		t.Context(),
		l,
		opts,
		component.Components{vpc, app},
		common.WithParseOptions(nil),
	)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(tmpDir)

	require.NoError(t, rnr.Run(t.Context(), l, testVenv(), opts, r))

	for _, unit := range []*component.Unit{vpc, app} {
		run, err := r.GetRun(unit.Path())
		require.NoError(t, err)
		assert.Equal(t, report.ResultExcluded, run.Result)

		// The output folder is pre-created for every unit, excluded or not.
		assert.DirExists(t, filepath.Dir(unit.OutputFile(tmpDir, opts.OutputFolder)))
	}
}

func TestRunnerRunReportsFailureAndAncestorEarlyExit(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", invalidHCL)
	db := newTestUnit(t, tmpDir, "db", "")
	app := newTestUnit(t, tmpDir, "app", "")

	db.AddDependency(vpc)
	app.AddDependency(db)

	opts := newRunOpts(t, tmpDir, "plan")

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(
		t.Context(),
		l,
		opts,
		component.Components{vpc, db, app},
	)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(tmpDir)

	err = rnr.Run(t.Context(), l, testVenv(), opts, r)
	require.Error(t, err)

	vpcRun, err := r.GetRun(vpc.Path())
	require.NoError(t, err)
	assert.Equal(t, report.ResultFailed, vpcRun.Result)

	// db exits early behind a failed dependency; app exits early behind an
	// early-exited one.
	for _, unit := range []*component.Unit{db, app} {
		run, err := r.GetRun(unit.Path())
		require.NoError(t, err)
		assert.Equal(t, report.ResultEarlyExit, run.Result)
		require.NotNil(t, run.Reason)
		assert.Equal(t, report.ReasonAncestorError, *run.Reason)
	}
}

func TestRunnerRunFailedUnitWithFailedDependencyIsEarlyExit(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", invalidHCL)
	app := newTestUnit(t, tmpDir, "app", invalidHCL)
	app.AddDependency(vpc)

	opts := newRunOpts(t, tmpDir, "plan")
	opts.IgnoreDependencyErrors = true
	opts.FailFast = false

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{vpc, app})
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(tmpDir)

	require.Error(t, rnr.Run(t.Context(), l, testVenv(), opts, r))

	// app is scheduled and fails on its own config, but the runner reclassifies
	// a failure behind a failed dependency as an early exit caused by it.
	appRun, err := r.GetRun(app.Path())
	require.NoError(t, err)
	assert.Equal(t, report.ResultEarlyExit, appRun.Result)
	require.NotNil(t, appRun.Cause)
	assert.Equal(t, report.Cause(filepath.Base(vpc.Path())), *appRun.Cause)
}

func TestRunnerRunWithoutReport(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", invalidHCL)

	opts := newRunOpts(t, tmpDir, "apply")
	opts.RunAllAutoApprove = true

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.Error(t, rnr.Run(t.Context(), l, testVenv(), opts, nil))
	assert.Contains(t, opts.TerraformCliArgs.Slice(), "-auto-approve")
}

func TestRunnerRunAuthProviderFailureFailsUnit(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", "")

	opts := newRunOpts(t, tmpDir, "plan")
	opts.AuthProviderCmd = "no-such-auth-provider"

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.Error(t, rnr.Run(t.Context(), l, testVenv(), opts, nil))
}

// TestRunnerRunUnitRunFailsWithoutBinary drives a unit all the way through
// config parsing into the unit run, where the fail-closed exec refuses to spawn
// Terraform. It covers the runner task's post-parse path without a real binary.
func TestRunnerRunUnitRunFailsWithoutBinary(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", "")

	opts := newRunOpts(t, tmpDir, "plan")

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.Error(t, rnr.Run(t.Context(), l, testVenv(), opts, nil))
}

func TestRunnerRunUnitTerraformBinaryOverride(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	vpc := newTestUnit(t, tmpDir, "vpc", "terraform_binary = \"custom-tofu\"\n")

	opts := newRunOpts(t, tmpDir, "plan")

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.ErrorContains(t, rnr.Run(t.Context(), l, testVenv(), opts, nil), "custom-tofu")
}

func TestRunnerRunEmptyStack(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	opts := newRunOpts(t, tmpDir, "show")

	l := thlogger.CreateLogger()

	rnr, err := runnerpool.NewRunnerPoolStack(t.Context(), l, opts, component.Components{})
	require.NoError(t, err)

	require.NoError(
		t,
		rnr.Run(t.Context(), l, testVenv(), opts, report.NewReport().WithWorkingDir(tmpDir)),
	)
}

// newTestUnit writes a unit directory under root and returns the matching component.
func newTestUnit(t *testing.T, root, name, hcl string) *component.Unit {
	t.Helper()

	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(hcl), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0o600))

	unit := component.NewUnit(dir).WithConfig(&config.TerragruntConfig{})
	unit.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: root})

	return unit
}

// newRunOpts returns stack options wired to root for the given Terraform command.
func newRunOpts(t *testing.T, root, command string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = root
	opts.RootWorkingDir = root
	opts.TerraformCommand = command
	opts.TerraformCliArgs = iacargs.New(command)

	return opts
}

// testVenv returns an in-memory venv backed by the real filesystem, since the
// runner reads configs written to a temp dir.
func testVenv() *venv.Venv {
	return venvtest.New().WithFS(vfs.NewOSFS())
}
