package commands_test

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCacheDir = errors.New("no user cache dir")

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

	require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, venv.MemVenv(), action))
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

	err := commands.RunAction(t.Context(), nil, logger.CreateLogger(), opts, venv.MemVenv(), action)
	require.Error(t, err)
	assert.True(t, panicreport.IsPanic(err), "returned error should be classified as a panic")

	msg, stack := panicreport.PanicDetails(err)
	assert.Equal(t, "action panic", msg)
	assert.Contains(t, string(stack), "run_action_test.go")
}

// TestRunActionProviderCacheStartupFailures pins that a provider cache server that cannot start fails the run before its action.
func TestRunActionProviderCacheStartupFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		venv     func(v *venv.Venv) *venv.Venv
		wantErr  error
		name     string
		cacheDir string
	}{
		{
			name:     "listener refused",
			cacheDir: "/virtual/provider-cache",
			wantErr:  venv.ErrNoListen,
		},
		{
			name: "cache dir unresolvable",
			venv: func(v *venv.Venv) *venv.Venv {
				return v.WithUserCacheDir(func() (string, error) { return "", errCacheDir })
			},
			wantErr: errCacheDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithGOOS("linux").WithHandler(fakeTerraformVersionHandler)
			if tc.venv != nil {
				v = tc.venv(v)
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.NoAutoProviderCacheDir = true
			opts.TFPath = "terraform"
			opts.ProviderCacheOptions.Enabled = true
			opts.ProviderCacheOptions.Dir = tc.cacheDir
			opts.ProviderCacheOptions.Hostname = "127.0.0.1"
			opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

			actionRan := false

			err := commands.RunAction(config.WithCaches(t.Context()), nil, logger.CreateLogger(), opts, v,
				func(context.Context, *clihelper.Context) error {
					actionRan = true

					return nil
				})
			require.ErrorIs(t, err, tc.wantErr)
			assert.False(t, actionRan, "the action must not run without the provider cache server")
		})
	}
}

// TestRunActionProgressReporter checks that the run reports a slow operation only with the experiment on.
func TestRunActionProgressReporter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		experiments  []string
		wantLogLines int
	}{
		{name: "experiment off"},
		{name: "experiment on", experiments: []string{experiment.SlowTaskReporting}, wantLogLines: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				logs := new(bytes.Buffer)
				l := logger.CreateLogger()
				l.SetOptions(log.WithOutput(logs), log.WithLevel(log.InfoLevel))

				opts := options.NewTerragruntOptions(vexec.NewOSExec())
				opts.NoAutoProviderCacheDir = true

				for _, name := range tc.experiments {
					require.NoError(t, opts.Experiments.EnableExperiment(name))
				}

				require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, venv.MemVenv(), slowAction(l)))
				assert.Equal(t, tc.wantLogLines, strings.Count(logs.String(), "\n"), logs)
			})
		})
	}
}

// TestRunActionProgressReporterAnimated checks that only a terminal with text logs at info level, outside CI, draws.
func TestRunActionProgressReporterAnimated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		change    func(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv)
		name      string
		wantDrawn bool
	}{
		{
			name:      "terminal",
			wantDrawn: true,
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

			synctest.Test(t, func(t *testing.T) {
				l, opts, v, term := terminalRun(t)

				if tc.change != nil {
					tc.change(l, opts, v)
				}

				require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, slowAction(l)))
				assert.Equal(t, tc.wantDrawn, strings.Contains(term.String(), "\x1b[J"), term.String())
			})
		})
	}
}

// TestRunActionLogsAroundProgressLine checks that log lines do not share a row with the progress line.
func TestRunActionLogsAroundProgressLine(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		l, opts, v, term := terminalRun(t)

		action := func(ctx context.Context, _ *clihelper.Context) error {
			return spinner.ShowAfter(ctx, l, spinner.Messages{
				Working: "working...",
				Done:    "finished",
			}, func() error {
				synctest.Sleep(1500 * time.Millisecond)
				l.Info("logged while working")
				synctest.Sleep(500 * time.Millisecond)

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

// TestRunActionProgressAfterPrompt checks that the progress line waits for a prompt to be answered.
func TestRunActionProgressAfterPrompt(t *testing.T) {
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
				synctest.Sleep(3 * time.Second)

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

// TestRunActionRestoresTerminal checks that the action leaves the streams and the logger as it found them.
func TestRunActionRestoresTerminal(t *testing.T) {
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

// slowAction returns an action that reports one operation slower than the reporting threshold.
func slowAction(l log.Logger) clihelper.ActionFunc {
	return func(ctx context.Context, _ *clihelper.Context) error {
		return spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(time.Second)

			return nil
		})
	}
}

func loggerOutput(l log.Logger) io.Writer {
	var output io.Writer

	l.SetOptions(log.WithOutputWrapper(func(current io.Writer) io.Writer {
		output = current

		return current
	}))

	return output
}

func terminalRun(t *testing.T) (log.Logger, *options.TerragruntOptions, *venv.Venv, *bytes.Buffer) {
	t.Helper()

	term := new(bytes.Buffer)

	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{
		"TERM": "xterm-256color",
		"LANG": "en_US.UTF-8",
	})
	v.Terminal = &venv.Terminal{
		StdinIsTTY:           func() bool { return true },
		StdoutIsTTY:          func() bool { return true },
		StderrIsTTY:          func() bool { return true },
		StderrHandlesEscapes: func() bool { return true },
		Width:                func() int { return 120 },
		ErrWidth:             func() int { return 120 },
	}

	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	l := log.New(log.WithLevel(log.InfoLevel), log.WithFormatter(formatter), log.WithOutput(term))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.SlowTaskReporting))

	return l, opts, v, term
}
