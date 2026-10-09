package commands_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunActionInstallsRunScopedCache pins the contract that RunAction
// installs the run-scoped cache on the context handed to the action.
// Without this, a future refactor could quietly drop the cache wiring and
// regress the optimization that 6019 introduced.
func TestRunActionInstallsRunScopedCache(t *testing.T) {
	t.Parallel()

	var (
		hasRunCmd    bool
		hasRepoRoots bool
	)

	action := func(ctx context.Context, _ *clihelper.Context) error {
		_, hasRunCmd = ctx.Value(cache.RunCmdCacheContextKey).(*cache.Cache[string])
		_, hasRepoRoots = ctx.Value(cache.RepoRootCacheContextKey).(*cache.RepoRootCache)

		return nil
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	l := logger.CreateLogger()

	require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, venvtest.New(), action))
	assert.True(t, hasRunCmd, "RunCmdCacheContextKey missing from action context")
	assert.True(t, hasRepoRoots, "RepoRootCacheContextKey missing from action context")
}

func TestRunActionReturnsReportableErrorOnActionPanic(t *testing.T) {
	t.Parallel()

	action := func(_ context.Context, _ *clihelper.Context) error {
		panic("action panic")
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	err := commands.RunAction(t.Context(), nil, logger.CreateLogger(), opts, venvtest.New(), action)
	require.Error(t, err)
	assert.True(t, panicreport.IsPanic(err), "returned error should be classified as a panic")

	msg, stack := panicreport.PanicDetails(err)
	assert.Equal(t, "action panic", msg)
	assert.Contains(t, string(stack), "run_action_test.go")
}

// TestRunActionInstallsProgressReporterForTheExperiment pins that the action
// context carries a progress reporter only when `slow-task-reporting` is on.
func TestRunActionInstallsProgressReporterForTheExperiment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		experiments  []string
		wantReporter bool
	}{
		{
			name: "experiment off",
		},
		{
			name:         "experiment on",
			experiments:  []string{experiment.SlowTaskReporting},
			wantReporter: true,
		},
		{
			name:        "another experiment on",
			experiments: []string{experiment.Symlinks},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var reporter *spinner.Reporter

			action := func(ctx context.Context, _ *clihelper.Context) error {
				reporter = spinner.ReporterFromContext(ctx)

				return nil
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.NoAutoProviderCacheDir = true

			for _, name := range tc.experiments {
				require.NoError(t, opts.Experiments.EnableExperiment(name))
			}

			require.NoError(t, commands.RunAction(t.Context(), nil, logger.CreateLogger(), opts, venvtest.New(), action))

			if !tc.wantReporter {
				assert.Nil(t, reporter)

				return
			}

			require.NotNil(t, reporter)
			assert.False(t, reporter.Animated(), "a run with no terminal reports as log lines")
		})
	}
}

// TestRunActionProgressReporterFollowsTheRun pins which runs draw a progress
// line: a terminal with human-readable logs does, and JSON logs, disabled
// logs, a quieter log level and a CI environment do not.
func TestRunActionProgressReporterFollowsTheRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		change       func(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv)
		name         string
		wantAnimated bool
	}{
		{
			name:         "terminal",
			wantAnimated: true,
		},
		{
			name: "JSON logs",
			change: func(_ log.Logger, opts *options.TerragruntOptions, _ *venv.Venv) {
				opts.JSONLogFormat = true
			},
		},
		{
			name: "logs disabled",
			change: func(l log.Logger, _ *options.TerragruntOptions, _ *venv.Venv) {
				l.Formatter().SetDisabledOutput(true)
			},
		},
		{
			name: "log level above info",
			change: func(l log.Logger, _ *options.TerragruntOptions, _ *venv.Venv) {
				l.SetOptions(log.WithLevel(log.WarnLevel))
			},
		},
		{
			name: "CI",
			change: func(_ log.Logger, _ *options.TerragruntOptions, v *venv.Venv) {
				v.Env["CI"] = "true"
			},
		},
		{
			name: "stderr is not a terminal",
			change: func(_ log.Logger, _ *options.TerragruntOptions, v *venv.Venv) {
				v.Terminal.StderrIsTTY = func() bool { return false }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, opts, v, _ := terminalRun(t)

			if tc.change != nil {
				tc.change(l, opts, v)
			}

			var reporter *spinner.Reporter

			action := func(ctx context.Context, _ *clihelper.Context) error {
				reporter = spinner.ReporterFromContext(ctx)

				return nil
			}

			require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))
			require.NotNil(t, reporter)
			assert.Equal(t, tc.wantAnimated, reporter.Animated())
		})
	}
}

// TestRunActionWritesLogsAroundTheProgressLine pins that a log line written
// while a progress line is on screen gets a row of its own.
func TestRunActionWritesLogsAroundTheProgressLine(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		l, opts, v, term := terminalRun(t)

		action := func(ctx context.Context, _ *clihelper.Context) error {
			return spinner.ShowAfter(ctx, l, spinner.Messages{
				Working: "working...",
				Done:    "finished",
			}, func() error {
				time.Sleep(1500 * time.Millisecond)
				l.Info("logged while working")
				time.Sleep(500 * time.Millisecond)

				return nil
			})
		}

		require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))

		stream := term.String()
		require.Contains(t, stream, "working... (1s)")

		var logged []string

		for line := range strings.SplitSeq(stream, "\n") {
			row := line[strings.LastIndex(line, "\r")+1:]
			if strings.Contains(row, "msg=") {
				logged = append(logged, row)
			}
		}

		require.Len(t, logged, 2)
		assert.Contains(t, logged[0], "logged while working")
		assert.NotContains(t, logged[0], "working...")
		assert.Contains(t, logged[1], "finished (2.0s)")
		assert.NotContains(t, logged[1], "working...")
	})
}

// TestRunActionDrawsProgressAfterAPromptIsAnswered pins that a prompt, which
// leaves the cursor in the middle of a row, holds the progress line back only
// until the user answers it.
func TestRunActionDrawsProgressAfterAPromptIsAnswered(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		l, opts, v, term := terminalRun(t)
		v.Stdin = strings.NewReader("y\n")

		action := func(ctx context.Context, _ *clihelper.Context) error {
			yes, err := shell.PromptUserForYesNo(ctx, l, v, "Create the bucket?", false)
			require.NoError(t, err)
			require.True(t, yes)

			return spinner.ShowAfter(ctx, l, spinner.Messages{
				Working: "Waiting for the bucket...",
				Done:    "The bucket is ready",
			}, func() error {
				time.Sleep(3 * time.Second)

				return nil
			})
		}

		require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))

		stream := term.String()
		prompt := strings.Index(stream, "Create the bucket? (y/n) ")
		frame := strings.Index(stream, "Waiting for the bucket... (1s)")

		require.GreaterOrEqual(t, prompt, 0, stream)
		assert.Greater(t, frame, prompt, "the progress line is drawn once the prompt is answered")
		assert.Contains(t, stream, "The bucket is ready (3.0s)")
	})
}

// TestRunActionGuardsTheTerminalOnlyForTheAction pins that the streams and the
// logger of the run are put back as they were once the action ends.
func TestRunActionGuardsTheTerminalOnlyForTheAction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		stdoutIsTTY   bool
		wantOutGuards bool
	}{
		{
			name:          "stdout is a terminal",
			stdoutIsTTY:   true,
			wantOutGuards: true,
		},
		{
			name: "stdout is redirected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, opts, v, term := terminalRun(t)
			v.Terminal.StdoutIsTTY = func() bool { return tc.stdoutIsTTY }

			writers := v.Writers

			var errGuarded, outGuarded, logGuarded bool

			action := func(context.Context, *clihelper.Context) error {
				errGuarded = v.Writers.ErrWriter != writers.ErrWriter
				outGuarded = v.Writers.Writer != writers.Writer
				logGuarded = loggerOutput(l) != io.Writer(term)

				return nil
			}

			require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))

			assert.True(t, errGuarded)
			assert.True(t, logGuarded)
			assert.Equal(t, tc.wantOutGuards, outGuarded)

			assert.Same(t, writers, v.Writers)
			assert.Same(t, term, loggerOutput(l))
		})
	}
}

// loggerOutput returns the writer l writes to.
func loggerOutput(l log.Logger) io.Writer {
	var output io.Writer

	l.SetOptions(log.WithOutputWrapper(func(current io.Writer) io.Writer {
		output = current

		return current
	}))

	return output
}

// terminalRun returns a logger, options and venv for a run with the experiment on whose stderr is a terminal, and the buffer standing in for it.
func terminalRun(t *testing.T) (log.Logger, *options.TerragruntOptions, *venv.Venv, *bytes.Buffer) {
	t.Helper()

	term := new(bytes.Buffer)

	v := venvtest.New().WithErrWriter(term).WithEnv(map[string]string{
		"TERM": "xterm-256color",
		"LANG": "en_US.UTF-8",
	})
	v.Terminal = &venv.Terminal{
		StdinIsTTY:  func() bool { return true },
		StdoutIsTTY: func() bool { return true },
		StderrIsTTY: func() bool { return true },
		Width:       func() int { return 120 },
		ErrWidth:    func() int { return 120 },
	}

	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	l := log.New(log.WithLevel(log.InfoLevel), log.WithFormatter(formatter), log.WithOutput(term))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.SlowTaskReporting))

	return l, opts, v, term
}
