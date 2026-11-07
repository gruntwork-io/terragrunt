package global_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileFlagsRegistration(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	profileFlags := global.NewProfileFlags(opts, nil)

	testCases := []struct {
		flagName string
		envName  string
	}{
		{flagName: global.ProfileCPUFlagName, envName: "TG_PROFILE_CPU"},
		{flagName: global.ProfileMemFlagName, envName: "TG_PROFILE_MEM"},
		{flagName: global.ProfileGoroutineFlagName, envName: "TG_PROFILE_GOROUTINE"},
		{flagName: global.ProfileDirFlagName, envName: "TG_PROFILE_DIR"},
	}

	for _, tc := range testCases {
		flag := profileFlags.Get(tc.flagName)
		require.NotNil(t, flag, "flag %s must be registered", tc.flagName)
		assert.Contains(t, flag.GetEnvVars(), tc.envName, "flag %s must declare the %s env var", tc.flagName, tc.envName)
	}
}

func TestProfileFlagsParse(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	err := global.NewProfileFlags(opts, nil).Parse([]string{
		"--profile-cpu=cpu.prof",
		"--profile-mem=mem.prof",
		"--profile-goroutine=goroutine.prof",
		"--profile-dir=profiles",
	}, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, "cpu.prof", opts.ProfileCPU)
	assert.Equal(t, "mem.prof", opts.ProfileMem)
	assert.Equal(t, "goroutine.prof", opts.ProfileGoroutine)
	assert.Equal(t, "profiles", opts.ProfileDir)
}

func TestProfileFlagsParseSpaceForm(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	err := global.NewProfileFlags(opts, nil).Parse([]string{"--profile-cpu", "cpu.prof"}, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, "cpu.prof", opts.ProfileCPU)
}

// TestGlobalFlagsSetOptions pins the option field or logger state each global flag writes.
func TestGlobalFlagsSetOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want any
		got  func(l log.Logger, opts *options.TerragruntOptions) any
		env  map[string]string
		name string
		args clihelper.Args
	}{
		{
			name: "working dir by name",
			args: clihelper.Args{"--working-dir", "/work"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.WorkingDir },
			want: "/work",
		},
		{
			name: "working dir by deprecated env var",
			env:  map[string]string{"TERRAGRUNT_WORKING_DIR": "/work"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.WorkingDir },
			want: "/work",
		},
		{
			name: "log disable silences logs and forwards tf stdout",
			args: clihelper.Args{"--log-disable"},
			got: func(l log.Logger, opts *options.TerragruntOptions) any {
				return []bool{l.Formatter().DisabledOutput(), opts.ForwardTFStdout}
			},
			want: []bool{true, true},
		},
		{
			name: "log disable set false keeps logs",
			args: clihelper.Args{"--log-disable=false"},
			got: func(l log.Logger, opts *options.TerragruntOptions) any {
				return []bool{l.Formatter().DisabledOutput(), opts.ForwardTFStdout}
			},
			want: []bool{false, false},
		},
		{
			name: "show abs paths",
			args: clihelper.Args{"--log-show-abs-paths"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.LogShowAbsPaths },
			want: true,
		},
		{
			name: "no color disables log colors",
			args: clihelper.Args{"--no-color"},
			got:  func(l log.Logger, _ *options.TerragruntOptions) any { return l.Formatter().DisabledColors() },
			want: true,
		},
		{
			name: "json log format",
			args: clihelper.Args{"--log-format", "json"},
			got: func(_ log.Logger, opts *options.TerragruntOptions) any {
				return []bool{opts.JSONLogFormat, opts.ForwardTFStdout}
			},
			want: []bool{true, false},
		},
		{
			name: "bare log format forwards tf stdout",
			args: clihelper.Args{"--log-format", "bare"},
			got: func(_ log.Logger, opts *options.TerragruntOptions) any {
				return []bool{opts.JSONLogFormat, opts.ForwardTFStdout}
			},
			want: []bool{false, true},
		},
		{
			name: "pretty log format changes neither option",
			args: clihelper.Args{"--log-format", "pretty"},
			got: func(_ log.Logger, opts *options.TerragruntOptions) any {
				return []bool{opts.JSONLogFormat, opts.ForwardTFStdout}
			},
			want: []bool{false, false},
		},
		{
			name: "non interactive by name",
			args: clihelper.Args{"--non-interactive"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.NonInteractive },
			want: true,
		},
		{
			name: "tf input false implies non interactive",
			env:  map[string]string{"TF_INPUT": "false"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.NonInteractive },
			want: true,
		},
		{
			name: "log level debug",
			args: clihelper.Args{"--log-level", "debug"},
			got:  func(l log.Logger, _ *options.TerragruntOptions) any { return l.Level() },
			want: log.DebugLevel,
		},
		{
			name: "log level by deprecated env var",
			env:  map[string]string{"TERRAGRUNT_LOG_LEVEL": "trace"},
			got:  func(l log.Logger, _ *options.TerragruntOptions) any { return l.Level() },
			want: log.TraceLevel,
		},
		{
			name: "experiment by name",
			args: clihelper.Args{"--experiment", experiment.OfflineCAS},
			got: func(_ log.Logger, opts *options.TerragruntOptions) any {
				return opts.Experiments.Evaluate(experiment.OfflineCAS)
			},
			want: true,
		},
		{
			name: "completed experiment is accepted",
			args: clihelper.Args{"--experiment", experiment.Stacks},
			got: func(_ log.Logger, opts *options.TerragruntOptions) any {
				return opts.Experiments.Evaluate(experiment.Stacks)
			},
			want: true,
		},
		{
			name: "telemetry trace exporter by env var",
			env:  map[string]string{"TG_TELEMETRY_TRACE_EXPORTER": "console"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.Telemetry.TraceExporter },
			want: "console",
		},
		{
			name: "telemetry trace exporter by misspelled deprecated env var",
			env:  map[string]string{"TERRAGRUNT_TELEMERTY_TRACE_EXPORTER": "console"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.Telemetry.TraceExporter },
			want: "console",
		},
		{
			name: "traceparent by env var",
			env:  map[string]string{"TRACEPARENT": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.Telemetry.TraceParent },
			want: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		},
		{
			name: "telemetry logs exporter by env var",
			env:  map[string]string{"TG_TELEMETRY_LOGS_EXPORTER": "otlpHttp"},
			got:  func(_ log.Logger, opts *options.TerragruntOptions) any { return opts.Telemetry.LogsExporter },
			want: "otlpHttp",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := newInfoLogger()
			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			env := tc.env
			if env == nil {
				env = map[string]string{}
			}

			require.NoError(t, parseAndRunActions(t, global.NewFlags(l, opts, nil), tc.args, env))
			assert.Equal(t, tc.want, tc.got(l, opts), "flag should write the parsed value")
		})
	}
}

// TestGlobalFlagsRejectInvalidValues pins that unknown names and malformed formats fail parsing.
func TestGlobalFlagsRejectInvalidValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args clihelper.Args
	}{
		{name: "unknown log level", args: clihelper.Args{"--log-level", "verbose"}},
		{name: "unknown log format", args: clihelper.Args{"--log-format", "xml"}},
		{name: "malformed custom log format", args: clihelper.Args{"--log-custom-format", "%unknown-placeholder"}},
		{name: "unknown experiment", args: clihelper.Args{"--experiment", "no-such-experiment"}},
		{name: "unknown strict control", args: clihelper.Args{"--strict-control", "no-such-control"}},
		{name: "unknown tip", args: clihelper.Args{"--no-tip", "no-such-tip"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			require.Error(t, parseAndRunActions(t, global.NewFlags(newInfoLogger(), opts, nil), tc.args, map[string]string{}))
		})
	}
}

// TestExperimentModeFlagEnablesOngoingExperiments pins that --experiment-mode turns on every ongoing experiment.
func TestExperimentModeFlagEnablesOngoingExperiments(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	ongoing := opts.Experiments.FilterByStatus(experiment.StatusOngoing)
	require.NotEmpty(t, ongoing, "at least one experiment should be ongoing")

	require.NoError(
		t,
		parseAndRunActions(
			t,
			global.NewFlags(newInfoLogger(), opts, nil),
			clihelper.Args{"--experiment-mode"},
			map[string]string{},
		),
	)

	for _, exp := range ongoing {
		assert.True(t, opts.Experiments.Evaluate(exp.Name), "experiment %s should be enabled", exp.Name)
	}
}

// TestStrictFlagsEnableControls pins that --strict-mode enables every active control and --strict-control enables one.
func TestStrictFlagsEnableControls(t *testing.T) {
	t.Parallel()

	t.Run("strict mode", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		active := opts.StrictControls.FilterByStatus(strict.ActiveStatus)
		require.NotEmpty(t, active, "at least one strict control should be active")

		require.NoError(
			t,
			parseAndRunActions(
				t,
				global.NewFlags(newInfoLogger(), opts, nil),
				clihelper.Args{"--strict-mode"},
				map[string]string{},
			),
		)

		for _, ctrl := range active {
			assert.True(t, ctrl.GetEnabled(), "control %s should be enabled", ctrl.GetName())
		}
	})

	t.Run("strict control", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())

		require.NoError(t, parseAndRunActions(
			t,
			global.NewFlags(newInfoLogger(), opts, nil),
			clihelper.Args{"--strict-control", controls.RootTerragruntHCL},
			map[string]string{},
		))

		ctrl := opts.StrictControls.Find(controls.RootTerragruntHCL)
		require.NotNil(t, ctrl)
		assert.True(t, ctrl.GetEnabled(), "named control should be enabled")
		assert.False(t, opts.StrictControls.Find(controls.BareInclude).GetEnabled(), "other controls should stay disabled")
	})
}

// TestTipFlagsDisableTips pins that --no-tips silences every tip and --no-tip silences only the named one.
func TestTipFlagsDisableTips(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		args         clihelper.Args
		wantSilenced []string
		wantShown    []string
	}{
		{
			name:         "no tips",
			args:         clihelper.Args{"--no-tips"},
			wantSilenced: []string{tips.DebuggingDocs, tips.WindowsSymlinkWarning},
		},
		{
			name:      "no tips set false",
			args:      clihelper.Args{"--no-tips=false"},
			wantShown: []string{tips.DebuggingDocs, tips.WindowsSymlinkWarning},
		},
		{
			name:         "no tip by name",
			args:         clihelper.Args{"--no-tip", tips.DebuggingDocs},
			wantSilenced: []string{tips.DebuggingDocs},
			wantShown:    []string{tips.WindowsSymlinkWarning},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			require.NoError(t, parseAndRunActions(t, global.NewFlags(newInfoLogger(), opts, nil), tc.args, map[string]string{}))

			// A tip's only observable effect is the line it logs, so a silenced tip logs nothing.
			for _, name := range tc.wantSilenced {
				assert.Empty(t, evaluateTip(t, opts.Tips, name), "tip %s should be silenced", name)
			}

			for _, name := range tc.wantShown {
				assert.NotEmpty(t, evaluateTip(t, opts.Tips, name), "tip %s should still show", name)
			}
		})
	}
}

// TestHelpVersionFlagsRenderOutput pins that --help prints app help and --version prints the version, both exiting successfully.
func TestHelpVersionFlagsRenderOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantOutput string
		args       clihelper.Args
	}{
		{name: "help", args: clihelper.Args{"--help"}, wantOutput: "test-app"},
		{name: "help alias", args: clihelper.Args{"-h"}, wantOutput: "test-app"},
		{name: "version", args: clihelper.Args{"--version"}, wantOutput: "1.2.3"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			app := clihelper.NewApp(map[string]string{})
			app.Name = "test-app"
			app.Version = "1.2.3"
			app.Writer = &out
			app.CustomAppHelpTemplate = "help for {{ .Name }}"
			app.CustomAppVersionTemplate = "{{ .Version }}"

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			helpVersionFlags := global.NewHelpVersionFlags(newInfoLogger(), opts)

			flagSet, err := helpVersionFlags.NewFlagSet(app.Name, map[string]string{}, returnErr)
			require.NoError(t, err)
			require.NoError(t, flagSet.Parse(tc.args))

			err = helpVersionFlags.RunActions(t.Context(), clihelper.NewAppContext(app, nil))

			var exitErr clihelper.ExitCoder
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, int(clihelper.ExitCodeSuccess), exitErr.ExitCode())
			assert.Contains(t, out.String(), tc.wantOutput, "rendered output should name the app or version")
		})
	}
}

// TestLogLevelFlagUsesPrefixedEnvVar pins that a command prefix lands between TG_ and the flag name.
func TestLogLevelFlagUsesPrefixedEnvVar(t *testing.T) {
	t.Parallel()

	l := newInfoLogger()
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	logLevelFlag := global.NewLogLevelFlag(l, opts, flags.Prefix{"run"})

	require.NoError(
		t,
		parseAndRunActions(t, clihelper.Flags{logLevelFlag}, nil, map[string]string{"TG_RUN_LOG_LEVEL": "warn"}),
	)
	assert.Equal(t, log.WarnLevel, l.Level())
}

// newInfoLogger returns a fresh info-level logger, so flags that mutate the logger never share state.
func newInfoLogger() log.Logger {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})

	return log.New(log.WithOutput(io.Discard), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))
}

// evaluateTip shows the named tip on a fresh logger and returns what it logged.
func evaluateTip(t *testing.T, allTips tips.Tips, name string) string {
	t.Helper()

	tip := allTips.Find(name)
	require.NotNil(t, tip, "tip %s should exist", name)

	var out bytes.Buffer

	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	tip.Evaluate(log.New(log.WithOutput(&out), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter)))

	return out.String()
}

// parseAndRunActions applies fs to a flag set the way a command does, parses args and env, and runs the actions of every flag that was set.
func parseAndRunActions(t *testing.T, fs clihelper.Flags, args clihelper.Args, env map[string]string) error {
	t.Helper()

	flagSet, err := fs.NewFlagSet("terragrunt", env, returnErr)
	if err != nil {
		return err
	}

	if err := flagSet.Parse(args); err != nil {
		return err
	}

	return fs.RunActions(t.Context(), &clihelper.Context{})
}

// returnErr is a flag error handler that passes every error through.
func returnErr(err error) error {
	return err
}
