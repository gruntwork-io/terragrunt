package runner_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// remoteStateErr is the unit output that makes the runner summarize remote state hints after a plan.
const remoteStateErr = "Error running plan: something failed: " +
	"Resource 'data.terraform_remote_state.vpc' does not have attribute\n"

// TestRunnerRun_ExcludedUnitsAreReported pins that excluded units are reported and keep their output folder.
func TestRunnerRun_ExcludedUnitsAreReported(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "")
	app := newTestUnit(t, v, memRoot, "app", "")

	vpc.SetExcluded(true)
	app.SetExcluded(true)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.OutputFolder = filepath.Join(memRoot, "out")

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{vpc, app},
	)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.NoError(t, rnr.Run(t.Context(), l, v, opts, r))

	for _, unit := range []*component.Unit{vpc, app} {
		run, err := r.GetRun(unit.Path())
		require.NoError(t, err)
		assert.Equal(t, report.ResultExcluded, run.Result, "%s is excluded", unit.Path())
		assert.True(
			t,
			vfs.IsDir(v.FS, filepath.Dir(unit.OutputFile(memRoot, opts.OutputFolder))),
			"the output folder is pre-created for every unit, excluded or not",
		)
	}
}

// TestRunnerRun_ReportsFailureAndAncestorEarlyExit pins that units behind a failed unit exit early.
func TestRunnerRun_ReportsFailureAndAncestorEarlyExit(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", invalidHCL)
	db := newTestUnit(t, v, memRoot, "db", "")
	app := newTestUnit(t, v, memRoot, "app", "")

	db.AddDependency(vpc)
	app.AddDependency(db)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{vpc, db, app},
	)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.Error(t, rnr.Run(t.Context(), l, v, opts, r))

	vpcRun, err := r.GetRun(vpc.Path())
	require.NoError(t, err)
	assert.Equal(t, report.ResultFailed, vpcRun.Result, "the unit with an unparsable config fails")

	for _, unit := range []*component.Unit{db, app} {
		run, err := r.GetRun(unit.Path())
		require.NoError(t, err)
		assert.Equal(
			t,
			report.ResultEarlyExit,
			run.Result,
			"%s exits early behind a failed dependency",
			unit.Path(),
		)
		require.NotNil(t, run.Reason)
		assert.Equal(t, report.ReasonAncestorError, *run.Reason)
	}
}

// TestRunnerRun_FailedUnitWithFailedDependencyIsEarlyExit pins that a failure behind a failed dependency is an early exit.
func TestRunnerRun_FailedUnitWithFailedDependencyIsEarlyExit(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", invalidHCL)
	app := newTestUnit(t, v, memRoot, "app", invalidHCL)
	app.AddDependency(vpc)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.IgnoreDependencyErrors = true
	opts.FailFast = false

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc, app})
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.Error(t, rnr.Run(t.Context(), l, v, opts, r))

	appRun, err := r.GetRun(app.Path())
	require.NoError(t, err)
	assert.Equal(t, report.ResultEarlyExit, appRun.Result, "a failure behind a failed dependency is an early exit")
	require.NotNil(t, appRun.Cause)
	assert.Equal(t, report.Cause(filepath.Base(vpc.Path())), *appRun.Cause, "the failed dependency is the cause")
}

// TestRunnerRun_WithoutReport pins that a run without a report still applies stack-level flags.
func TestRunnerRun_WithoutReport(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", invalidHCL)

	opts := newStackOpts(t, memRoot, tf.CommandNameApply)
	opts.RunAllAutoApprove = true

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.Error(t, rnr.Run(t.Context(), l, v, opts, nil))
	assert.Contains(
		t,
		opts.TerraformCliArgs.Slice(),
		"-auto-approve",
		"--auto-approve is inserted for apply runs",
	)
}

// TestRunnerRun_AuthProviderFailureFailsUnit pins that a unit fails when its auth provider command cannot run.
func TestRunnerRun_AuthProviderFailureFailsUnit(t *testing.T) {
	t.Parallel()

	const authProviderCmd = "no-such-auth-provider"

	v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == authProviderCmd {
			return vexec.Result{Err: vexec.ErrNoSpawn}
		}

		return vexec.Result{Stdout: []byte(tfVersionOutput + "\n")}
	})

	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.AuthProviderCmd = authProviderCmd

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.ErrorIs(t, rnr.Run(t.Context(), l, v, opts, nil), vexec.ErrNoSpawn)
}

// TestRunnerRun_UnitRunFailsWithoutBinary pins the error a parsed unit hits when no binary may be spawned.
func TestRunnerRun_UnitRunFailsWithoutBinary(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.ErrorIs(t, rnr.Run(t.Context(), l, v, opts, nil), vexec.ErrNoSpawn)
}

// TestRunnerRun_UnitTerraformBinaryOverride pins that the unit run spawns a unit's terraform_binary.
func TestRunnerRun_UnitTerraformBinaryOverride(t *testing.T) {
	t.Parallel()

	v, invocations := recordingVenv("", "")
	vpc := newTestUnit(t, v, memRoot, "vpc", `terraform_binary = "custom-tofu"`)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)
	require.NoError(t, rnr.Run(t.Context(), l, v, opts, nil))

	plan := commandInvocation(t, invocations(), tf.CommandNamePlan)
	assert.Equal(t, "custom-tofu", plan.Name, "the plan runs the unit's terraform_binary")
}

// TestRunnerRun_EmptyStack pins that running a stack without units is a no-op.
func TestRunnerRun_EmptyStack(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	opts := newStackOpts(t, memRoot, tf.CommandNameShow)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{})
	require.NoError(t, err)

	require.NoError(
		t,
		rnr.Run(t.Context(), l, v, opts, report.NewReport().WithWorkingDir(memRoot)),
	)
}

// TestRunnerRun_SyncsUnitCliArgs pins the arguments a unit run passes to OpenTofu/Terraform.
func TestRunnerRun_SyncsUnitCliArgs(t *testing.T) {
	t.Parallel()

	planFile := filepath.Join(memRoot, "out", "vpc", tf.TerraformPlanFile)

	testCases := []struct {
		discoveryArgs []string
		name          string
		command       string
		outputFolder  string
		stackArgs     []string
		expected      []string
		unexpected    []string
		jsonOutput    bool
	}{
		{
			name:      "stack args are cloned into the unit",
			command:   tf.CommandNamePlan,
			stackArgs: []string{tf.CommandNamePlan, "-lock=false"},
			expected:  []string{"-lock=false"},
		},
		{
			name:          "discovery args are merged with stack flags",
			command:       tf.CommandNamePlan,
			discoveryArgs: []string{"-lock=false"},
			stackArgs:     []string{tf.CommandNamePlan, "-refresh=false"},
			expected:      []string{"-lock=false", "-refresh=false"},
		},
		{
			name:         "plan gets an out flag",
			command:      tf.CommandNamePlan,
			stackArgs:    []string{tf.CommandNamePlan},
			outputFolder: filepath.Join(memRoot, "out"),
			expected:     []string{"-out=" + planFile},
		},
		{
			name:         "show gets the plan file as an argument",
			command:      tf.CommandNameShow,
			stackArgs:    []string{tf.CommandNameShow},
			outputFolder: filepath.Join(memRoot, "out"),
			expected:     []string{planFile},
		},
		{
			name:       "json output folder yields the default plan file",
			command:    tf.CommandNamePlan,
			stackArgs:  []string{tf.CommandNamePlan},
			jsonOutput: true,
			expected:   []string{"-out=" + tf.TerraformPlanFile},
		},
		{
			name:         "an existing plan file is left alone",
			command:      tf.CommandNamePlan,
			stackArgs:    []string{tf.CommandNamePlan, "-out=existing.tfplan"},
			outputFolder: filepath.Join(memRoot, "out"),
			expected:     []string{"-out=existing.tfplan"},
			unexpected:   []string{"-out=" + planFile},
		},
		{
			name:       "no plan file for other commands",
			command:    tf.CommandNameApply,
			stackArgs:  []string{tf.CommandNameApply},
			unexpected: []string{"-out=" + planFile},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, invocations := recordingVenv("", "")
			vpc := newTestUnit(t, v, memRoot, "vpc", "")

			if len(tc.discoveryArgs) > 0 {
				vpc.SetDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: memRoot,
					Cmd:        tc.command,
					Args:       tc.discoveryArgs,
				})
			}

			opts := newStackOpts(t, memRoot, tc.command)
			opts.TerraformCliArgs = iacargs.New(tc.stackArgs...)
			opts.OutputFolder = tc.outputFolder
			opts.RunAllAutoApprove = true

			if tc.jsonOutput {
				opts.JSONOutputFolder = filepath.Join(memRoot, "json")
			}

			l := thlogger.CreateLogger()

			rnr, err := runner.NewFromComponents(
				t.Context(),
				l,
				opts,
				component.Components{vpc},
			)
			require.NoError(t, err)
			require.NoError(t, rnr.Run(t.Context(), l, v, opts, nil))

			if tc.jsonOutput {
				exists, err := vfs.FileExists(v.FS, vpc.OutputJSONFile(memRoot, opts.JSONOutputFolder))
				require.NoError(t, err)
				assert.True(t, exists, "the JSON plan output is written through the venv filesystem")
			}

			args := commandInvocation(t, invocations(), tc.command).Args

			for _, want := range tc.expected {
				assert.Contains(t, args, want, "%s is passed to the unit run", want)
			}

			for _, unwanted := range tc.unexpected {
				assert.NotContains(t, args, unwanted, "%s is not passed to the unit run", unwanted)
			}
		})
	}
}

// TestRunnerRun_PlanWithRemoteStateErrors pins that a plan reporting missing remote state attributes still completes.
func TestRunnerRun_PlanWithRemoteStateErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		unitConfig *config.TerragruntConfig
		name       string
		stderr     string
		withDep    bool
	}{
		{
			name: "no output",
		},
		{
			name:   "unrelated output",
			stderr: "Error running plan: some unrelated failure\n",
		},
		{
			name:   "remote state error without dependencies",
			stderr: remoteStateErr,
		},
		{
			name:    "remote state error with unresolved dependency paths",
			stderr:  remoteStateErr,
			withDep: true,
		},
		{
			name:    "remote state error with configured dependency paths",
			stderr:  remoteStateErr,
			withDep: true,
			unitConfig: &config.TerragruntConfig{
				Dependencies: &config.ModuleDependencies{Paths: []string{"../vpc"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, _ := recordingVenv("", tc.stderr)

			app := newTestUnit(t, v, memRoot, "app", "")
			if tc.unitConfig != nil {
				app = app.WithConfig(tc.unitConfig)
				app.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: memRoot})
			}

			components := component.Components{app}

			if tc.withDep {
				vpc := newTestUnit(t, v, memRoot, "vpc", "")
				app.AddDependency(vpc)

				components = append(components, vpc)
			}

			opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

			l := thlogger.CreateLogger()

			rnr, err := runner.NewFromComponents(t.Context(), l, opts, components)
			require.NoError(t, err)
			require.NoError(t, rnr.Run(t.Context(), l, v, opts, nil))
		})
	}
}

// The in-memory exec's exit code is invisible to util.GetExitCode, so the fake plan fails with an ExitStatus error.
func TestRunnerRun_ReportsUnitOutcome(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantResult report.Result
		exitCode   int
		wantErr    bool
	}{
		{
			name:       "plan with changes",
			exitCode:   tf.DetailedExitCodeChanges,
			wantResult: report.ResultSucceeded,
		},
		{
			name:       "plan that fails",
			exitCode:   tf.DetailedExitCodeError,
			wantResult: report.ResultFailed,
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				if slices.Contains(inv.Args, tf.FlagNameDetailedExitCode) {
					return vexec.Result{Err: exitStatusErr{code: tc.exitCode}}
				}

				return vexec.Result{Stdout: []byte(tfVersionOutput + "\n")}
			})

			vpc := newTestUnit(t, v, memRoot, "vpc", "")

			opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
			opts.TerraformCliArgs = iacargs.New(tf.CommandNamePlan, tf.FlagNameDetailedExitCode)

			l := thlogger.CreateLogger()

			rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
			require.NoError(t, err)

			exitCodes := tf.NewDetailedExitCodeMap()
			ctx := tf.ContextWithDetailedExitCode(t.Context(), exitCodes)
			r := report.NewReport().WithWorkingDir(memRoot)

			err = rnr.Run(ctx, l, v, opts, r)
			assert.Equal(t, tc.wantErr, err != nil, "run error: %v", err)

			run, err := r.GetRun(vpc.Path())
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, run.Result)
			assert.Equal(t, tc.exitCode, exitCodes.Get(vpc.Path()), "the unit exit code reaches the caller")
		})
	}
}

func TestRunnerRun_OutputFolderCannotBeCreated(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.OutputFolder = filepath.Join(memRoot, "out")

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	faulty := *v
	faulty.FS = &faultFS{FS: v.FS, op: faultMkdirAll, under: opts.OutputFolder}

	require.ErrorIs(t, rnr.Run(t.Context(), l, &faulty, opts, nil), errInjected)
}

// The report refuses relative paths, so those units are left out and the run goes on.
func TestRunnerRun_RelativeUnitPathsAreLeftOutOfTheReport(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)

	reported := newTestUnit(t, v, memRoot, "db", "")
	reported.SetExcluded(true)

	excluded := component.NewUnit("vpc").WithConfig(&config.TerragruntConfig{})
	excluded.SetExcluded(true)

	failing := component.NewUnit("app").WithConfig(&config.TerragruntConfig{})

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{reported, excluded, failing},
	)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.Error(t, rnr.Run(t.Context(), l, v, opts, r), "the unit without a config file fails")

	require.Len(t, r.Runs, 1, "the report only records absolute unit paths")
	assert.Equal(t, reported.Path(), r.Runs[0].Path)
}

func TestRunnerRun_UnitOptionsCannotBeBuilt(t *testing.T) {
	t.Parallel()

	badSource := "no-slashes-here"

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "").WithConfig(&config.TerragruntConfig{
		Terraform: &config.TerraformConfig{Source: &badSource},
	})

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.Source = "/local/modules"

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	var target config.ParsingModulePathError

	require.ErrorAs(t, rnr.Run(t.Context(), l, v, opts, nil), &target)
}

// Two units run in each case, so a warning given per unit instead of per run would be counted twice.
func TestRunnerRun_WarnsOnLocalStateWithGitRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		unitBody string
		mainTF   string
		ref      string
		want     int
	}{
		{
			name: "no remote state",
			ref:  "HEAD~1",
			want: 1,
		},
		{
			name: "local backend",
			unitBody: `remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}`,
			mainTF: `terraform {
  backend "local" {}
}`,
			ref:  "HEAD~1",
			want: 1,
		},
		{
			name: "remote backend",
			unitBody: `remote_state {
  backend = "http"
  disable_init = true
  config = {
    address = "http://127.0.0.1:1/state"
  }
}`,
			mainTF: `terraform {
  backend "http" {}
}`,
			ref: "HEAD~1",
		},
		{
			name: "no git ref",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(tfVersionOutput)

			names := []string{"vpc", "app"}
			units := make(component.Components, 0, len(names))

			for _, name := range names {
				dir := writeUnit(t, v, memRoot, name, tc.unitBody)
				require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(dir, "main.tf"), []byte(tc.mainTF), 0o644))

				unit := component.NewUnit(dir).WithConfig(&config.TerragruntConfig{})
				unit.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: memRoot, Ref: tc.ref})

				units = append(units, unit)
			}

			opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

			var out bytes.Buffer

			l := thlogger.CreateLogger().WithOptions(log.WithOutput(&out))

			rnr, err := runner.NewFromComponents(t.Context(), l, opts, units)
			require.NoError(t, err)
			require.NoError(t, rnr.Run(t.Context(), l, v, opts, nil))

			assert.Equal(
				t,
				tc.want,
				strings.Count(out.String(), "do not have a remote_state configuration"),
				"the warning is given at most once per run",
			)
		})
	}
}

// No engine clients are set up, so releasing the unit's engine fails.
func TestRunnerRun_EngineShutdownFailureKeepsUnitResult(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.IacEngine))

	var out bytes.Buffer

	l := thlogger.CreateLogger().WithOptions(log.WithOutput(&out))

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.NoError(t, rnr.Run(t.Context(), l, v, opts, nil))
	assert.Contains(t, out.String(), "Error shutting down engine for unit", "no engine clients were set up to release")
}

// Output without a trailing newline stays buffered until the final flush, which is the write that fails.
func TestRunnerRun_OutputFlushFailureFailsUnit(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write failed")

	v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "-version") {
			return vexec.Result{Stdout: []byte(tfVersionOutput + "\n")}
		}

		return vexec.Result{Stdout: []byte("partial line")}
	}).WithWriter(&failingWriter{err: writeErr})

	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	require.ErrorIs(t, rnr.Run(t.Context(), l, v, opts, nil), writeErr)
}

func TestRunnerRun_RecordsTaskOutcomeOnSpan(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "")
	app := newTestUnit(t, v, memRoot, "app", invalidHCL)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc, app})
	require.NoError(t, err)

	buf, tlm := newConsoleTelemeter(t)
	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	require.Error(t, rnr.Run(ctx, l, v, opts, nil))
	require.NoError(t, tlm.Shutdown(ctx))

	outcomes := map[any]any{}
	for _, span := range spansNamed(decodeSpans(t, buf), "runner_pool_task") {
		outcomes[span.Attrs["unit_path"]] = span.Attrs["outcome"]
	}

	assert.Equal(t, map[any]any{vpc.Path(): "succeeded", app.Path(): "failed"}, outcomes)
}

func TestRunnerRun_JSONOutputCannotBeWritten(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	vpc := newTestUnit(t, v, memRoot, "vpc", "")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.JSONOutputFolder = filepath.Join(memRoot, "json")

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{vpc})
	require.NoError(t, err)

	faulty := *v
	faulty.FS = &faultFS{FS: v.FS, op: faultMkdirAll, under: opts.JSONOutputFolder}

	require.ErrorIs(t, rnr.Run(t.Context(), l, &faulty, opts, nil), errInjected)
}

func TestUnitRunnerRun_WithoutOptions(t *testing.T) {
	t.Parallel()

	unitRunner := runner.NewUnitRunner(component.NewUnit(memRoot))

	require.NoError(t, unitRunner.Run(t.Context(), thlogger.CreateLogger(), venvtest.New(), nil, nil, nil, nil))
	assert.Equal(t, runner.Running, unitRunner.Status)
}

// newTestUnit writes a unit directory into the in-memory filesystem of v and returns the matching component.
func newTestUnit(t *testing.T, v *venv.Venv, root, name, hcl string) *component.Unit {
	t.Helper()

	unit := component.NewUnit(writeUnit(t, v, root, name, hcl)).
		WithConfig(&config.TerragruntConfig{})
	unit.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: root})

	return unit
}

// recordingVenv returns an in-memory venv answering every invocation with stdout and stderr, and its recording.
func recordingVenv(stdout, stderr string) (*venv.Venv, func() []vexec.Invocation) {
	var (
		mu       sync.Mutex
		recorded []vexec.Invocation
	)

	v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		recorded = append(recorded, inv)

		return vexec.Result{Stdout: []byte(stdout), Stderr: []byte(stderr)}
	})

	return v, func() []vexec.Invocation {
		mu.Lock()
		defer mu.Unlock()

		return slices.Clone(recorded)
	}
}

// exitStatusErr fails a fake OpenTofu/Terraform process with code, as an exited process does.
type exitStatusErr struct {
	code int
}

func (e exitStatusErr) Error() string { return "exit status " + strconv.Itoa(e.code) }

func (e exitStatusErr) ExitStatus() (int, error) { return e.code, nil }

// commandInvocation returns the first invocation of command.
func commandInvocation(t *testing.T, invocations []vexec.Invocation, command string) vexec.Invocation {
	t.Helper()

	for _, inv := range invocations {
		if len(inv.Args) > 0 && inv.Args[0] == command {
			return inv
		}
	}

	require.FailNowf(t, "no invocation recorded", "expected a %s invocation, got %v", command, invocations)

	return vexec.Invocation{}
}
