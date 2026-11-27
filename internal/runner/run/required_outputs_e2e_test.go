package run_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const stateWithVPCID = `{"vpc_id": {"sensitive": false, "type": "string", "value": "vpc-1"}}`

var afterApplyHook = runcfg.Hook{
	Name:     "after-apply",
	Commands: []string{"apply"},
	Execute:  []string{"step-after"},
	If:       true,
}

func TestRunChecksRequiredOutputsAfterApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env              map[string]string
		name             string
		command          string
		args             []string
		required         []string
		expectedCommands []string
		expectedMissing  []string
	}{
		{
			name:             "an apply that leaves a required output out of the state fails",
			command:          "apply",
			args:             []string{"-auto-approve"},
			required:         []string{"subnet_ids", "vpc_id"},
			expectedCommands: []string{"apply", "output"},
			expectedMissing:  []string{"subnet_ids"},
		},
		{
			name:             "an apply that writes every required output passes",
			command:          "apply",
			args:             []string{"-auto-approve"},
			required:         []string{"vpc_id"},
			expectedCommands: []string{"apply", "output"},
		},
		{
			name:             "an apply with -refresh-only reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-refresh-only"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -help reads no outputs",
			command:          "apply",
			args:             []string{"-help"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "a unit with no required outputs reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "plan reads no outputs",
			command:          "plan",
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"plan"},
		},
		{
			name:             "destroy reads no outputs",
			command:          "destroy",
			args:             []string{"-auto-approve"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"destroy"},
		},
		{
			name:             "an apply with -destroy reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-destroy"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -target reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-target=terraform_data.a"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -exclude reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-exclude=terraform_data.a"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -target-file reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-target-file=targets"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -exclude-file reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-exclude-file=excludes"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -state reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-state=custom.tfstate"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -state-out reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve", "-state-out=custom.tfstate"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply of a saved plan file reads no outputs",
			command:          "apply",
			args:             []string{"tfplan"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply of a saved plan file with the name of a subcommand reads no outputs",
			command:          "apply",
			args:             []string{"show"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply of a saved plan file in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			env:              map[string]string{"TF_CLI_ARGS_apply": "tfplan"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with a flag terminator in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			env:              map[string]string{"TF_CLI_ARGS_apply": "-no-color -- -partial.plan"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -target in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_apply": "-target=terraform_data.a"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with a quoted -target in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_apply": "'-target=terraform_data.a'"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with a quote in a flag name of TF_CLI_ARGS reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS": `-tar"get"=terraform_data.a`},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with a backslash in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_apply": `-tar\get=terraform_data.a`},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with TF_CLI_ARGS_apply in another letter case reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_APPLY": "-state-out=custom.tfstate"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with a TF_CLI_ARGS variable in lower case reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"tf_cli_args_output": "-state=custom.tfstate"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -exclude in TF_CLI_ARGS reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS": "-no-color -exclude=terraform_data.a"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "an apply with -destroy in TF_CLI_ARGS_apply reads no outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_apply": "-destroy"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "-destroy on the command line wins over -destroy=false in TF_CLI_ARGS_apply",
			command:          "apply",
			args:             []string{"-auto-approve", "-destroy"},
			env:              map[string]string{"TF_CLI_ARGS_apply": "-destroy=false"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply"},
		},
		{
			name:             "-destroy=false in TF_CLI_ARGS wins over -destroy in TF_CLI_ARGS_apply",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS_apply": "-destroy", "TF_CLI_ARGS": "-destroy=false"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply", "output"},
			expectedMissing:  []string{"subnet_ids"},
		},
		{
			name:             "-destroy=false on the command line wins over -destroy in TF_CLI_ARGS",
			command:          "apply",
			args:             []string{"-auto-approve", "-destroy=false"},
			env:              map[string]string{"TF_CLI_ARGS": "-destroy"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply", "output"},
			expectedMissing:  []string{"subnet_ids"},
		},
		{
			name:             "an apply with other flags in TF_CLI_ARGS checks the outputs",
			command:          "apply",
			args:             []string{"-auto-approve"},
			env:              map[string]string{"TF_CLI_ARGS": "-input=false"},
			required:         []string{"subnet_ids"},
			expectedCommands: []string{"apply", "output"},
			expectedMissing:  []string{"subnet_ids"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			rec := &recorder{}
			v := outputsVenv(rec, vexec.Result{}, vexec.Result{Stdout: []byte(stateWithVPCID)})
			maps.Copy(v.Env, tc.env)

			err := run.Run(
				t.Context(),
				logger.CreateLogger(),
				v,
				newRunE2EOpts(t, s, tc.command, tc.args...),
				report.NewReport(),
				&runcfg.RunConfig{RequiredOutputs: tc.required},
				creds.NewGetter(),
			)

			require.Equal(t, tc.expectedCommands, commandNames(rec))

			if tc.expectedMissing == nil {
				require.NoError(t, err)

				return
			}

			var missing run.RequiredOutputsMissingError

			require.ErrorAs(t, err, &missing)
			require.Equal(t, tc.expectedMissing, missing.Names)
			require.Equal(t, s.configPath, missing.ConfigPath)
		})
	}
}

func TestRunReadsNoRequiredOutputsAfterAFailedApply(t *testing.T) {
	t.Parallel()

	errApply := errors.New("apply failed")

	s := setupRunE2EScaffold(t)
	rec := &recorder{}

	opts := newRunE2EOpts(t, s, "apply", "-auto-approve")
	opts.AutoRetry = false

	err := run.Run(
		t.Context(),
		logger.CreateLogger(),
		outputsVenv(rec, vexec.Result{Err: errApply}, vexec.Result{Stdout: []byte(stateWithVPCID)}),
		opts,
		report.NewReport(),
		&runcfg.RunConfig{RequiredOutputs: []string{"vpc_id"}},
		creds.NewGetter(),
	)

	require.ErrorIs(t, err, errApply)
	require.Equal(t, []string{"apply"}, commandNames(rec))
}

func TestRunReadsNoRequiredOutputsOfAUnitThatExcludeKeepsFromTheRun(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}

	cfg := &runcfg.RunConfig{
		RequiredOutputs: []string{"vpc_id"},
		Exclude:         runcfg.ExcludeConfig{If: true, NoRun: true, Actions: []string{"apply"}},
	}

	err := run.Run(
		t.Context(),
		logger.CreateLogger(),
		outputsVenv(rec, vexec.Result{}, vexec.Result{Stdout: []byte(`{}`)}),
		newRunE2EOpts(t, s, "apply", "-auto-approve"),
		report.NewReport(),
		cfg,
		creds.NewGetter(),
	)

	require.NoError(t, err)
	require.Empty(t, commandNames(rec))
}

func TestRunAppliesTheErrorsBlockToTheRequiredOutputsCheck(t *testing.T) {
	t.Parallel()

	applyFailure := vexec.Result{Err: errors.New("apply failed")}
	ignoreApply := []*runcfg.IgnoreBlock{{Label: "apply", IgnorableErrors: []string{".*apply failed.*"}}}

	testCases := []struct {
		name             string
		errors           runcfg.ErrorsConfig
		expectedCommands []string
		script           execScript
	}{
		{
			name: "a failed read does not start a retry of the apply",
			errors: runcfg.ErrorsConfig{Retry: []*runcfg.RetryBlock{
				{Label: "read", RetryableErrors: []string{".*transient output read.*"}, MaxAttempts: 2},
			}},
			script: execScript{
				commands: []vexec.Result{{}},
				outputs:  []vexec.Result{{Err: errors.New("transient output read")}},
			},
			expectedCommands: []string{"apply", "output", "step-after"},
		},
		{
			name:   "an ignored failure of the apply reads no outputs",
			errors: runcfg.ErrorsConfig{Ignore: ignoreApply},
			script: execScript{
				commands: []vexec.Result{applyFailure},
				outputs:  []vexec.Result{{Stdout: []byte(stateWithVPCID)}},
			},
			expectedCommands: []string{"apply"},
		},
		{
			name: "an ignore block can ignore the missing outputs",
			errors: runcfg.ErrorsConfig{Ignore: []*runcfg.IgnoreBlock{
				{Label: "outputs", IgnorableErrors: []string{".*does not have after apply.*"}},
			}},
			script: execScript{
				commands: []vexec.Result{{}},
				outputs:  []vexec.Result{{Stdout: []byte(`{}`)}},
			},
			expectedCommands: []string{"apply", "output", "step-after"},
		},
		{
			name: "a retry of the apply reads the outputs one time, after the attempt that succeeds",
			errors: runcfg.ErrorsConfig{Retry: []*runcfg.RetryBlock{
				{Label: "apply", RetryableErrors: []string{".*apply failed.*"}, MaxAttempts: 2},
			}},
			script: execScript{
				commands: []vexec.Result{applyFailure, {}},
				outputs:  []vexec.Result{{Stdout: []byte(stateWithVPCID)}},
			},
			expectedCommands: []string{"apply", "apply", "output", "step-after"},
		},
		{
			name: "an ignored failure of a second attempt drops the outputs of the first attempt",
			errors: runcfg.ErrorsConfig{
				Retry: []*runcfg.RetryBlock{
					{Label: "hook", RetryableErrors: []string{".*hook failed.*"}, MaxAttempts: 2},
				},
				Ignore: ignoreApply,
			},
			script: execScript{
				commands: []vexec.Result{{}, applyFailure},
				outputs:  []vexec.Result{{Stdout: []byte(`{}`)}},
				hook:     vexec.Result{Err: errors.New("hook failed")},
			},
			expectedCommands: []string{"apply", "output", "step-after", "apply"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			rec := &recorder{}

			opts := newRunE2EOpts(t, s, "apply", "-auto-approve", "-replace=terraform_data.a")
			opts.AutoRetry = true

			cfg := &runcfg.RunConfig{
				RequiredOutputs: []string{"vpc_id"},
				Errors:          tc.errors,
				Terraform:       runcfg.TerraformConfig{AfterHooks: []runcfg.Hook{afterApplyHook}},
			}

			err := run.Run(
				t.Context(),
				logger.CreateLogger(),
				scriptedVenv(rec, &tc.script),
				opts,
				report.NewReport(),
				cfg,
				creds.NewGetter(),
			)

			require.NoError(t, err)
			require.Equal(t, tc.expectedCommands, commandNames(rec))
		})
	}
}

func TestRunChecksRequiredOutputsAfterAnIgnoredFailureOfAnAfterHook(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}

	cfg := &runcfg.RunConfig{
		RequiredOutputs: []string{"vpc_id"},
		Errors: runcfg.ErrorsConfig{Ignore: []*runcfg.IgnoreBlock{
			{Label: "hook", IgnorableErrors: []string{".*hook failed.*"}},
		}},
		Terraform: runcfg.TerraformConfig{AfterHooks: []runcfg.Hook{afterApplyHook}},
	}

	script := &execScript{
		commands: []vexec.Result{{}},
		outputs:  []vexec.Result{{Stdout: []byte(`{}`)}},
		hook:     vexec.Result{Err: errors.New("hook failed")},
	}

	err := run.Run(
		t.Context(),
		logger.CreateLogger(),
		scriptedVenv(rec, script),
		newRunE2EOpts(t, s, "apply", "-auto-approve"),
		report.NewReport(),
		cfg,
		creds.NewGetter(),
	)

	var missing run.RequiredOutputsMissingError

	require.ErrorAs(t, err, &missing)
	require.Equal(t, []string{"vpc_id"}, missing.Names)
	require.Equal(t, []string{"apply", "output", "step-after"}, commandNames(rec))
}

func TestRunPassesWhenTheReadOfTheRequiredOutputsFails(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		output vexec.Result
	}{
		{name: "the output command fails", output: vexec.Result{Err: errors.New("output failed")}},
		{name: "the output command prints no JSON", output: vexec.Result{Stdout: []byte("no outputs")}},
		{
			name:   "the output command prints a JSON log record before the outputs",
			output: vexec.Result{Stdout: []byte(`{"@level": "info", "@message": "log"}` + "\n" + stateWithVPCID)},
		},
		{
			name:   "the output command prints an empty object in a warning before the outputs",
			output: vexec.Result{Stdout: []byte(`The file "{}-missing.tfrc" does not exist.` + "\n" + stateWithVPCID)},
		},
		{
			name:   "the output command prints an object without the fields of an output before the outputs",
			output: vexec.Result{Stdout: []byte(`{"record": {"message": "log"}}` + "\n" + stateWithVPCID)},
		},
		{
			name:   "the output command prints a warning before an empty object",
			output: vexec.Result{Stdout: []byte("Warning: Deprecated Parameter\n{}\n")},
		},
		{
			name:   "the output command prints an empty object and the outputs on one line",
			output: vexec.Result{Stdout: []byte("{} " + stateWithVPCID)},
		},
		{
			name:   "the output command prints a brace without JSON before the outputs",
			output: vexec.Result{Stdout: []byte(`The file "{-missing.tfrc" does not exist.` + "\n" + stateWithVPCID)},
		},
		{
			name:   "the output command prints an output without its type",
			output: vexec.Result{Stdout: []byte(`{"vpc_id": {"sensitive": false, "value": "vpc-1"}}`)},
		},
		{
			name:   "the output command prints an output without its sensitive flag",
			output: vexec.Result{Stdout: []byte(`{"vpc_id": {"type": "string", "value": "vpc-1"}}`)},
		},
		{
			name:   "the output command prints an output without its value",
			output: vexec.Result{Stdout: []byte(`{"vpc_id": {"sensitive": false, "type": "string"}}`)},
		},
		{name: "the output command prints null", output: vexec.Result{Stdout: []byte("null")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}

			require.NoError(t, runApplyWithOutputResult(t, rec, tc.output))
			require.Equal(t, []string{"apply", "output"}, commandNames(rec))
		})
	}
}

func TestRunReadsRequiredOutputsBesideAWarning(t *testing.T) {
	t.Parallel()

	err := runApplyWithOutputResult(
		t,
		&recorder{},
		vexec.Result{Stdout: []byte("Warning: Deprecated Parameter\n" + stateWithVPCID + "\n")},
	)

	var missing run.RequiredOutputsMissingError

	require.ErrorAs(t, err, &missing)
	require.Equal(t, []string{"subnet_ids"}, missing.Names)
}

func TestRunFailsWhenTheOutputCommandPrintsAnEmptyObjectFirst(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stdout string
	}{
		{name: "the empty object is alone", stdout: "{}\n"},
		{name: "a warning follows the empty object", stdout: "{}\nWarning: Deprecated Parameter\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := runApplyWithOutputResult(t, &recorder{}, vexec.Result{Stdout: []byte(tc.stdout)})

			var missing run.RequiredOutputsMissingError

			require.ErrorAs(t, err, &missing)
			require.Equal(t, []string{"subnet_ids"}, missing.Names)
		})
	}
}

func TestRunReadsRequiredOutputsWithABareOutputCommandInTheEnvironmentOfTheApply(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := outputsVenv(rec, vexec.Result{}, vexec.Result{Stdout: []byte(stateWithVPCID)})
	v.Env["TF_CLI_ARGS_output"] = "-raw"
	v.Env["TF_WORKSPACE"] = "blue"

	opts := newRunE2EOpts(t, s, "apply", "-auto-approve")
	opts.ProfileDir = t.TempDir()

	cfg := &runcfg.RunConfig{
		RequiredOutputs: []string{"vpc_id"},
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Name:     "apply",
					Commands: []string{"apply"},
					EnvVars:  map[string]string{"TF_CLI_ARGS": "-input=false"},
				},
				{
					Name:      "output",
					Commands:  []string{"output"},
					Arguments: []string{"-raw", "vpc_id"},
					EnvVars:   map[string]string{"TF_WORKSPACE": "default"},
				},
			},
		},
	}

	require.NoError(
		t,
		run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()),
	)

	calls := rec.snapshot()
	require.Len(t, calls, 2)

	hasProfile := func(entry string) bool { return strings.HasPrefix(entry, tf.EnvNameTofuCPUProfile+"=") }
	hasCLIArgs := func(entry string) bool { return strings.HasPrefix(entry, tf.EnvNameTFCLIArgs) }

	require.True(t, slices.ContainsFunc(calls[0].Env, hasProfile))
	require.Contains(t, calls[0].Env, "TF_CLI_ARGS=-input=false")

	require.Equal(t, []string{"output", "-json"}, calls[1].Args)
	require.Contains(t, calls[1].Env, "TF_WORKSPACE=blue")
	require.Contains(t, calls[1].Env, "TF_INPUT=0")
	require.False(t, slices.ContainsFunc(calls[1].Env, hasProfile))
	require.False(t, slices.ContainsFunc(calls[1].Env, hasCLIArgs))
}

func TestRunReadsRequiredOutputsBeforeTheVariableFilesAreRemoved(t *testing.T) {
	t.Parallel()

	var (
		mu         sync.Mutex
		nullVars   string
		duringRead bool
	)

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if !slices.Contains(inv.Args, tf.CommandNameOutput) {
			return vexec.Result{}
		}

		mu.Lock()
		defer mu.Unlock()

		nullVars = filepath.Join(inv.Dir, run.NullTFVarsFile)
		_, err := os.Stat(nullVars)
		duringRead = err == nil

		return vexec.Result{Stdout: []byte(stateWithVPCID)}
	}

	require.NoError(
		t,
		run.Run(
			t.Context(),
			logger.CreateLogger(),
			venv.MemVenv().WithExec(vexec.NewMemExec(handler)).WithFS(vfs.NewOSFS()),
			newRunE2EOpts(t, setupRunE2EScaffold(t), "apply", "-auto-approve"),
			report.NewReport(),
			&runcfg.RunConfig{RequiredOutputs: []string{"vpc_id"}, Inputs: map[string]any{"passphrase": nil}},
			creds.NewGetter(),
		),
	)

	mu.Lock()
	defer mu.Unlock()

	require.True(t, duringRead)
	require.NoFileExists(t, nullVars)
}

func TestRunReadsRequiredOutputsBeforeTheAfterHooksOfApply(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}

	cfg := &runcfg.RunConfig{
		RequiredOutputs: []string{"subnet_ids"},
		Terraform:       runcfg.TerraformConfig{AfterHooks: []runcfg.Hook{afterApplyHook}},
	}

	err := run.Run(
		t.Context(),
		logger.CreateLogger(),
		outputsVenv(rec, vexec.Result{}, vexec.Result{Stdout: []byte(stateWithVPCID)}),
		newRunE2EOpts(t, s, "apply", "-auto-approve"),
		report.NewReport(),
		cfg,
		creds.NewGetter(),
	)

	var missing run.RequiredOutputsMissingError

	require.ErrorAs(t, err, &missing)
	require.Equal(t, []string{"apply", "output", "step-after"}, commandNames(rec))
}

// runApplyWithOutputResult runs an apply of a unit that requires subnet_ids and returns the error of the run.
func runApplyWithOutputResult(t *testing.T, rec *recorder, result vexec.Result) error {
	t.Helper()

	opts := newRunE2EOpts(t, setupRunE2EScaffold(t), "apply", "-auto-approve")
	opts.AutoRetry = false

	return run.Run(
		t.Context(),
		logger.CreateLogger(),
		outputsVenv(rec, vexec.Result{}, result),
		opts,
		report.NewReport(),
		&runcfg.RunConfig{RequiredOutputs: []string{"subnet_ids"}},
		creds.NewGetter(),
	)
}

// execScript holds the results of the IaC command and of the output command in call order, and the result of a hook.
type execScript struct {
	commands []vexec.Result
	outputs  []vexec.Result
	hook     vexec.Result
}

// outputsVenv returns a venv whose exec records in rec and answers the output command with outputs in order.
func outputsVenv(rec *recorder, command vexec.Result, outputs ...vexec.Result) *venv.Venv {
	return scriptedVenv(rec, &execScript{commands: []vexec.Result{command}, outputs: outputs})
}

// scriptedVenv returns a venv whose exec records in rec and answers from script, where the last result repeats.
func scriptedVenv(rec *recorder, script *execScript) *venv.Venv {
	var mu sync.Mutex

	commands, reads := 0, 0

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		rec.record(&inv)

		if inv.Name != "tofu" {
			return script.hook
		}

		mu.Lock()
		defer mu.Unlock()

		if !slices.Contains(inv.Args, tf.CommandNameOutput) {
			result := script.commands[min(commands, len(script.commands)-1)]
			commands++

			return result
		}

		result := script.outputs[min(reads, len(script.outputs)-1)]
		reads++

		return result
	}

	return venv.MemVenv().WithExec(vexec.NewMemExec(handler)).WithFS(vfs.NewOSFS())
}

// commandNames returns the IaC command of each invocation that rec holds, or the program name of a hook.
func commandNames(rec *recorder) []string {
	calls := rec.snapshot()
	names := make([]string, 0, len(calls))

	for _, call := range calls {
		if call.Name != "tofu" {
			names = append(names, call.Name)

			continue
		}

		names = append(names, call.Args[0])
	}

	return names
}
