package spinner_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testThreshold = 50 * time.Millisecond
	firstFrame    = "\u2801"
	gutter        = " "
)

var animatedOptions = spinner.Options{Threshold: testThreshold, LogsForHumans: true}

// TestShowAfterFastCompletion checks that an operation faster than the threshold logs nothing.
func TestShowAfterFastCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, logs.String())
	})
}

// TestShowAfterSlowCompletion checks that a slow operation logs the working line, then the done line.
func TestShowAfterSlowCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "completed",
		}, func() error {
			synctest.Sleep(200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 2)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
		assert.Contains(t, lines[1], `msg=completed`)
	})
}

// TestShowAfterSlowError_NoDoneLog checks that a failed operation logs no done line.
func TestShowAfterSlowError_NoDoneLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		errBoom := errors.New("boom")

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			synctest.Sleep(200 * time.Millisecond)

			return errBoom
		})

		require.ErrorIs(t, err, errBoom)

		lines := logLines(logs)
		require.Len(t, lines, 1)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
	})
}

// TestShowAfterErrorPropagation checks that the error of a fast operation is returned.
func TestShowAfterErrorPropagation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		errBoom := errors.New("boom")

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			return errBoom
		})

		require.ErrorIs(t, err, errBoom)
		assert.Empty(t, logs.String())
	})
}

// TestShowAfterContextCancelledAfterTimeout checks that a drawn operation cancelled midway logs no done line.
func TestShowAfterContextCancelledAfterTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		err := terminalReporter(term, 120).ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(150 * time.Millisecond)
			cancel()
			synctest.Sleep(50 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, term.String(), "working...")
		assert.Empty(t, logs.String())
	})
}

// TestShowAfterSpinnerThenLog checks that the working message is drawn and only the done message is logged.
func TestShowAfterSpinnerThenLog(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := terminalReporter(term, 120).ShowAfter(t.Context(), l, spinner.Messages{
			Working: "creating worktree...",
			Done:    "created worktree",
		}, func() error {
			synctest.Sleep(500 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, term.String(), "creating worktree...")

		lines := logLines(logs)
		require.Len(t, lines, 1)
		assert.Contains(t, lines[0], "msg=\"created worktree\"")
	})
}

// TestShowAfterSpinnerNotShownWhenFast checks that a fast operation is neither drawn nor logged.
func TestShowAfterSpinnerNotShownWhenFast(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := terminalReporter(term, 120).ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, term.String())
		assert.Empty(t, logs.String())
	})
}

// TestWriter checks that the progress line is drawn only on an interactive stderr.
func TestWriter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tty       bool
		wantDrawn bool
	}{
		{name: "interactive", tty: true, wantDrawn: true},
		{name: "not a terminal"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				errWriter := new(syncBuffer)

				v := terminalVenv(errWriter, 120)
				v.Terminal.StderrIsTTY = func() bool { return tc.tty }

				err := spinner.New(v, animatedOptions).ShowAfter(t.Context(), newLogger(new(bytes.Buffer)), spinner.Messages{
					Working: "working...",
					Done:    "done",
				}, func() error {
					synctest.Sleep(time.Second)

					return nil
				})

				require.NoError(t, err)
				assert.Equal(t, tc.wantDrawn, errWriter.String() != "")
			})
		})
	}
}

// TestShowAfterElapsedTimeShown checks that the done line has the elapsed time.
func TestShowAfterElapsedTimeShown(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(1200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, logs.String(), `msg="finished (1.2s)"`)
	})
}

// TestShowAfterNonInteractiveKeepalive checks that the working line is repeated every 30 seconds.
func TestShowAfterNonInteractiveKeepalive(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(61 * time.Second)

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 4)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
		assert.Contains(t, lines[1], `msg="working... (30s elapsed)"`)
		assert.Contains(t, lines[2], `msg="working... (60s elapsed)"`)
		assert.Contains(t, lines[3], `msg="finished (61.0s)"`)
	})
}

// TestShowAfterKeepaliveCountsFromStart checks that the 30-second lines are counted from the start of the operation.
func TestShowAfterKeepaliveCountsFromStart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		want    []string
		started bool
	}{
		{
			name: "announced once slow",
			want: []string{
				"msg=working...",
				`msg="working... (30s elapsed)"`,
				`msg="working... (60s elapsed)"`,
				`msg="finished (61.0s)"`,
			},
		},
		{
			name:    "announced by the caller",
			started: true,
			want: []string{
				`msg="working... (30s elapsed)"`,
				`msg="working... (60s elapsed)"`,
				`msg="finished (61.0s)"`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				logs := new(bytes.Buffer)
				l := newLogger(logs)

				reporter := spinner.New(venv.MemVenv(), spinner.Options{Threshold: spinner.DefaultThreshold})

				err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
					Working: "working...",
					Done:    "finished",
					Started: tc.started,
				}, func() error {
					synctest.Sleep(61 * time.Second)

					return nil
				})

				require.NoError(t, err)

				lines := logLines(logs)
				require.Len(t, lines, len(tc.want), lines)

				for i, want := range tc.want {
					assert.True(t, strings.HasSuffix(lines[i], want), lines[i])
				}
			})
		})
	}
}

// TestShowAfterStartedLogsWorkingOnce checks that a working line already logged by the caller is not logged again.
func TestShowAfterStartedLogsWorkingOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
			Started: true,
		}, func() error {
			synctest.Sleep(31 * time.Second)

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 2)
		assert.Contains(t, lines[0], `msg="working... (30s elapsed)"`)
		assert.Contains(t, lines[1], `msg="finished (31.0s)"`)
	})
}

// TestShowAfterContextCancelledBeforeTimeout checks that an operation cancelled before the threshold logs nothing.
func TestShowAfterContextCancelledBeforeTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := logReporter().ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			synctest.Sleep(100 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, logs.String())
	})
}

// TestShowAfterContextCancelledNonInteractive checks that a logged operation cancelled midway logs no done line.
func TestShowAfterContextCancelledNonInteractive(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		err := logReporter().ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(150 * time.Millisecond)
			cancel()

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 1)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
	})
}

// TestShowReportsFromTheStart checks that Show reports the operation without waiting for the threshold.
func TestShowReportsFromTheStart(t *testing.T) {
	t.Parallel()

	for range 200 {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().Show(t.Context(), l, spinner.Messages{
			Working: "waiting",
			Done:    "approved",
		}, func() error {
			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 2)
		assert.Contains(t, lines[0], `msg=waiting`)
		assert.Contains(t, lines[1], `msg=approved`)
	}
}

// TestShowAfterWithoutReporter checks that without a reporter the operation only runs.
func TestShowAfterWithoutReporter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx := spinner.ContextWithLogOnly(t.Context())

		ran := false

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			synctest.Sleep(time.Minute)

			ran = true

			return nil
		})

		require.NoError(t, err)
		assert.True(t, ran)
		assert.Empty(t, logs.String())
	})
}

// TestShowAfterUsesContextReporter checks that the package ShowAfter reports through the reporter on the context.
func TestShowAfterUsesContextReporter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx := spinner.ContextWithReporter(t.Context(), logReporter())

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			synctest.Sleep(200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Len(t, logLines(logs), 2)
	})
}

// TestContextWithLogOnlyNeverDraws checks that a log-only context writes log lines and never draws.
func TestContextWithLogOnlyNeverDraws(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		reporter := terminalReporter(term, 120)
		require.True(t, reporter.Animated())

		ctx := spinner.ContextWithLogOnly(spinner.ContextWithReporter(t.Context(), reporter))

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			synctest.Sleep(time.Second)

			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, term.String())
		assert.Len(t, logLines(logs), 2)
	})
}

// TestProgressLineFormat checks that the line shows the frame, the message and the elapsed time.
func TestProgressLineFormat(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := terminalReporter(term, 120)
		l := newLogger(reporter.Guard(term))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Cloning repository...",
			Done:    "Cloned repository",
		}, func() error {
			synctest.Sleep(2500 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)

		frames := drawnLines(term.Bytes())
		require.NotEmpty(t, frames)
		assert.Equal(t, gutter+firstFrame+" Cloning repository... (0s)", frames[0])
		assert.True(t, slices.ContainsFunc(frames, func(frame string) bool {
			return strings.HasSuffix(frame, " Cloning repository... (2s)")
		}), frames)

		screen := screenRows(term.Bytes(), 120)
		require.Len(t, screen, 1)
		assert.Contains(t, screen[0], `msg="Cloned repository (2.5s)"`)
	})
}

// TestProgressLineNeverWraps checks that a long message is cut to the terminal width.
func TestProgressLineNeverWraps(t *testing.T) {
	t.Parallel()

	const (
		width = 80
		url   = "git::https://github.com/gruntwork-io/terraform-aws-service-catalog.git?ref=v0.118.4"
	)

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := terminalReporter(term, width)
		l := newLogger(reporter.Guard(term))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Downloading source from " + url + "...",
			Done:    "Downloaded",
		}, func() error {
			synctest.Sleep(3 * time.Second)

			return nil
		})

		require.NoError(t, err)

		frames := drawnLines(term.Bytes())
		require.NotEmpty(t, frames)

		for _, frame := range frames {
			assert.LessOrEqual(t, runewidth.StringWidth(frame), width-1, frame)
		}

		assert.True(t, strings.HasSuffix(frames[0], "... (0s)"), frames[0])

		screen := screenRows(term.Bytes(), width)
		require.Len(t, screen, 1)
		assert.Contains(t, screen[0], "msg=\"Downloaded (3.0s)\"")
	})
}

// TestProgressLineControlChars checks that tabs, line breaks and wide characters keep the line on one row.
func TestProgressLineControlChars(t *testing.T) {
	t.Parallel()

	const width = 40

	testCases := []struct {
		name    string
		working string
		want    string
	}{
		{
			name:    "tab and line breaks",
			working: "Reading a\tb\nc\r\nd",
			want:    gutter + firstFrame + " Reading a b c  d (0s)",
		},
		{
			name:    "wide characters are counted as two columns",
			working: "Downloading " + strings.Repeat("\uff21\uff22", 10),
			want:    gutter + firstFrame + " Downloading " + strings.Repeat("\uff21\uff22", 4) + "... (0s)",
		},
		{
			name:    "combining marks take no column",
			working: "Cloning cafe\u0301 repo 0123456789 0123456789 0123456789",
			want:    gutter + firstFrame + " Cloning cafe\u0301 repo 0123456789... (0s)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				term := new(syncBuffer)
				reporter := terminalReporter(term, width)
				l := newLogger(reporter.Guard(term))

				err := reporter.Show(t.Context(), l, spinner.Messages{
					Working: tc.working,
					Done:    "done",
				}, func() error {
					synctest.Sleep(50 * time.Millisecond)

					return nil
				})

				require.NoError(t, err)

				frames := drawnLines(term.Bytes())
				require.NotEmpty(t, frames)
				assert.Equal(t, tc.want, frames[0])
				assert.LessOrEqual(t, runewidth.StringWidth(frames[0]), width-1)
			})
		})
	}
}

// TestProgressLineShared checks that running operations share one line and logs are written above it.
func TestProgressLineShared(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := terminalReporter(term, 200)
		l := newLogger(reporter.Guard(term))
		ctx := t.Context()

		var wg sync.WaitGroup

		errs := make([]error, 2)

		for i, wait := range []time.Duration{2500 * time.Millisecond, 3500 * time.Millisecond} {
			wg.Go(func() {
				errs[i] = reporter.ShowAfter(ctx, l, spinner.Messages{
					Working: "Downloading source...",
					Done:    "Downloaded source",
				}, func() error {
					time.Sleep(wait)

					return nil
				})
			})
		}

		synctest.Sleep(1550 * time.Millisecond)
		l.Info("unit-c: tofu init finished")

		during := screenRows(term.Bytes(), 200)

		wg.Wait()

		require.NoError(t, errs[0])
		require.NoError(t, errs[1])

		require.Len(t, during, 2)
		assert.True(t, strings.HasPrefix(during[0], "time="), during[0])
		assert.Contains(t, during[0], `msg="unit-c: tofu init finished"`)
		assert.Equal(t, gutter+"\u2812 Downloading source... (1s) +1 more", during[1])

		after := screenRows(term.Bytes(), 200)
		require.Len(t, after, 3)

		for _, row := range after {
			assert.True(t, strings.HasPrefix(row, "time="), row)
			assert.NotContains(t, row, "Downloading source...")
		}
	})
}

// TestProgressLineClearedOnPanic checks that the line is cleared when the operation panics.
func TestProgressLineClearedOnPanic(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := terminalReporter(term, 120)
		l := newLogger(reporter.Guard(term))

		var err error

		require.Panics(t, func() {
			err = reporter.ShowAfter(t.Context(), l, spinner.Messages{
				Working: "working...",
				Done:    "done",
			}, func() error {
				synctest.Sleep(200 * time.Millisecond)
				panic("boom")
			})
		})
		require.NoError(t, err)

		drawn := len(term.Bytes())
		require.NotZero(t, drawn)
		assert.Empty(t, screenRows(term.Bytes(), 120))

		synctest.Sleep(5 * time.Second)

		assert.Len(t, term.Bytes(), drawn)
	})
}

// TestProgressFrames checks that the frames match the locale and the terminal.
func TestProgressFrames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env       map[string]string
		name      string
		goos      string
		wantFrame string
	}{
		{
			name:      "linux with a UTF-8 locale",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "en_US.UTF-8"},
			wantFrame: firstFrame,
		},
		{
			name:      "linux with the C locale",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "C"},
			wantFrame: "|",
		},
		{
			name:      "linux with no locale",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm"},
			wantFrame: "|",
		},
		{
			name:      "LC_ALL wins over LANG",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "en_US.UTF-8", "LC_ALL": "POSIX"},
			wantFrame: "|",
		},
		{
			name:      "LC_CTYPE wins over LANG",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "C", "LC_CTYPE": "en_US.utf8"},
			wantFrame: firstFrame,
		},
		{
			name:      "macOS terminal",
			goos:      "darwin",
			env:       map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"},
			wantFrame: firstFrame,
		},
		{
			name:      "windows console",
			goos:      "windows",
			env:       map[string]string{},
			wantFrame: "|",
		},
		{
			name:      "windows console ignores the locale",
			goos:      "windows",
			env:       map[string]string{"LANG": "en_US.UTF-8"},
			wantFrame: "|",
		},
		{
			name:      "windows terminal",
			goos:      "windows",
			env:       map[string]string{"WT_SESSION": "c4a9a1b2"},
			wantFrame: firstFrame,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				term := new(syncBuffer)

				v := terminalVenv(term, 120)
				v = v.WithEnv(tc.env)
				v = v.WithGOOS(tc.goos)

				reporter := spinner.New(v, animatedOptions)
				require.True(t, reporter.Animated())

				l := newLogger(reporter.Guard(term))

				err := reporter.Show(t.Context(), l, spinner.Messages{
					Working: "working",
					Done:    "done",
				}, func() error {
					synctest.Sleep(50 * time.Millisecond)

					return nil
				})

				require.NoError(t, err)

				frames := drawnLines(term.Bytes())
				require.NotEmpty(t, frames)
				assert.Equal(t, gutter+tc.wantFrame+" working (0s)", frames[0])
			})
		})
	}
}

// TestProgressFramesUnicode checks that the Unicode frames draw T, then G, in one cell.
func TestProgressFramesUnicode(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := terminalReporter(term, 120)
		l := newLogger(reporter.Guard(term))

		err := reporter.Show(t.Context(), l, spinner.Messages{
			Working: "working",
			Done:    "done",
		}, func() error {
			synctest.Sleep(2550 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)

		var drawn []string

		for _, line := range drawnLines(term.Bytes()) {
			frame, _, _ := strings.Cut(strings.TrimPrefix(line, gutter), " ")
			if len(drawn) == 0 || drawn[len(drawn)-1] != frame {
				drawn = append(drawn, frame)
			}
		}

		assert.Equal(t, []string{
			firstFrame,
			"\u2802",
			"\u2804",
			"\u280d",
			"\u2816",
			"\u281e",
			"\u2834",
			"\u2820",
			"\u2800",
			"\u2809",
			"\u2812",
			"\u281b",
			"\u2836",
			"\u2824",
			"\u2800",
			firstFrame,
		}, drawn)
	})
}

// TestShowAfterSpinnerWriteFailure checks that a failed terminal write is logged once and the operation completes.
func TestShowAfterSpinnerWriteFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.DebugLevel), log.WithOutput(logs))

		reporter := terminalReporter(failingWriter{}, 120)
		require.True(t, reporter.Animated())

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			synctest.Sleep(2 * time.Second)

			return nil
		})

		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(logs.String(), "Progress is no longer drawn on the terminal"))
		assert.Contains(t, logs.String(), "terminal is gone")
		assert.Contains(t, logs.String(), `msg="finished (2.0s)"`)
	})
}

// TestProgressLineTooNarrow checks that the line is not drawn once the terminal is too narrow.
func TestProgressLineTooNarrow(t *testing.T) {
	t.Parallel()

	const narrow = 10

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		start := time.Now()

		v := terminalVenv(term, 120)
		v.Terminal.ErrWidth = func() int {
			if time.Since(start) < time.Second {
				return 120
			}

			return narrow
		}

		reporter := spinner.New(v, animatedOptions)
		l := newLogger(new(bytes.Buffer))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			synctest.Sleep(500 * time.Millisecond)

			require.Len(t, screenRows(term.Bytes(), 120), 1)

			synctest.Sleep(510 * time.Millisecond)

			before := len(term.Bytes())

			synctest.Sleep(2 * time.Second)

			after := term.Bytes()[before:]
			assert.Empty(t, drawnLines(after), "no frame is drawn on a terminal narrower than the minimum")

			for _, segment := range drawnSegments(after) {
				assert.LessOrEqual(t, runewidth.StringWidth(segment), narrow-1, segment)
			}

			return nil
		})

		require.NoError(t, err)
	})
}

// TestProgressLineWaitsForPrompt checks that the line is held back until a prompt is answered.
func TestProgressLineWaitsForPrompt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		wantRows      []string
		inputIsTTY    bool
		wantDrawAfter bool
	}{
		{
			name:          "answer typed on a terminal",
			inputIsTTY:    true,
			wantDrawAfter: true,
		},
		{
			name: "answer read from a pipe",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				term := new(syncBuffer)

				v := terminalVenv(term, 120)
				v.Terminal.StdinIsTTY = func() bool { return tc.inputIsTTY }

				reporter := spinner.New(v, animatedOptions)
				guarded := reporter.Guard(term)
				l := newLogger(guarded)
				ctx := spinner.ContextWithReporter(t.Context(), reporter)

				err := spinner.ShowAfter(ctx, l, spinner.Messages{
					Working: "working...",
					Done:    "done",
				}, func() error {
					synctest.Sleep(300 * time.Millisecond)

					_, err := guarded.Write([]byte("Create the bucket? (y/n) "))
					require.NoError(t, err)

					synctest.Sleep(time.Second)

					assert.Equal(t, []string{"Create the bucket? (y/n)"}, screenRows(term.Bytes(), 120))

					if tc.inputIsTTY {
						_, err = term.Write([]byte("y\n"))
						require.NoError(t, err)
					}

					spinner.InputRead(ctx)

					before := len(term.Bytes())

					synctest.Sleep(time.Second)

					assert.Equal(t, tc.wantDrawAfter, len(drawnLines(term.Bytes()[before:])) > 0)

					return nil
				})

				require.NoError(t, err)
			})
		})
	}
}

// TestGuardLoggerRestore checks that the function GuardLogger returns puts back the original logger output.
func TestGuardLoggerRestore(t *testing.T) {
	t.Parallel()

	output := new(bytes.Buffer)
	l := newLogger(output)

	restore := terminalReporter(new(syncBuffer), 120).GuardLogger(l)

	assert.NotSame(t, output, loggerOutput(l))

	restore()

	assert.Same(t, output, loggerOutput(l))
}

// TestProgressLineShrinkNoWrap checks that the line fits a terminal that shrinks during the operation.
func TestProgressLineShrinkNoWrap(t *testing.T) {
	t.Parallel()

	const narrow = 30

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		start := time.Now()

		v := terminalVenv(term, 120)
		v.Terminal.ErrWidth = func() int {
			if time.Since(start) < time.Second {
				return 120
			}

			return narrow
		}

		reporter := spinner.New(v, animatedOptions)
		l := newLogger(reporter.Guard(term))

		var before int

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Downloading source from git::https://github.com/acme/modules.git?ref=v1.2.3...",
			Done:    "done",
		}, func() error {
			synctest.Sleep(1010 * time.Millisecond)

			before = len(term.Bytes())

			synctest.Sleep(2 * time.Second)

			return nil
		})

		require.NoError(t, err)

		after := drawnSegments(term.Bytes()[before:])
		require.NotEmpty(t, drawnLines(term.Bytes()[before:]), "the line is still drawn after the terminal shrank")

		for _, segment := range after {
			assert.LessOrEqual(t, runewidth.StringWidth(segment), narrow-1, segment)
		}
	})
}

// TestProgressLineShrinkNoLeftovers checks that no rows are left behind after the terminal shrinks.
func TestProgressLineShrinkNoLeftovers(t *testing.T) {
	t.Parallel()

	const (
		wide   = 120
		narrow = 30
	)

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)

		var width atomic.Int64

		width.Store(wide)

		v := terminalVenv(term, wide)
		v.Terminal.ErrWidth = func() int { return int(width.Load()) }

		reporter := spinner.New(v, animatedOptions)
		l := newLogger(reporter.Guard(term))

		var shrunk int

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Downloading source from git::https://github.com/acme/modules.git?ref=v1.2.3...",
			Done:    "Downloaded source",
		}, func() error {
			synctest.Sleep(1550 * time.Millisecond)

			shrunk = len(term.Bytes())

			width.Store(narrow)

			synctest.Sleep(time.Second)

			return nil
		})

		require.NoError(t, err)

		screen := newScreen(wide)
		screen.write(term.Bytes()[:shrunk])
		require.Len(t, screen.rows(), 1)

		screen.resize(narrow)
		require.Greater(t, len(screen.rows()), 1, "the terminal re-wraps the wide line onto more rows")

		screen.write(term.Bytes()[shrunk:])

		rows := screen.rows()
		require.NotEmpty(t, rows)
		assert.True(t, strings.HasPrefix(rows[0], "time="), rows)
		assert.Contains(t, strings.Join(rows, ""), `msg="Downloaded source (`)
		assert.NotContains(t, strings.Join(rows, ""), "acme/modules", "no part of the progress line is left on screen")
	})
}

type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (buffer *syncBuffer) Write(p []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	return buffer.buf.Write(p)
}

func (buffer *syncBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	return bytes.Clone(buffer.buf.Bytes())
}

func (buffer *syncBuffer) String() string {
	return string(buffer.Bytes())
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("terminal is gone")
}

func logReporter() *spinner.Reporter {
	return spinner.New(venv.MemVenv(), spinner.Options{Threshold: testThreshold})
}

func terminalReporter(out io.Writer, width int) *spinner.Reporter {
	return spinner.New(terminalVenv(out, width), animatedOptions)
}

// terminalVenv returns a venv whose stderr is out, a UTF-8 terminal of width columns.
func terminalVenv(out io.Writer, width int) *venv.Venv {
	v := venv.MemVenv().
		WithErrWriter(out).
		WithEnv(map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"}).
		WithGOOS("linux")

	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return width }

	return v
}

func newLogger(out io.Writer) log.Logger {
	return log.New(log.WithLevel(log.InfoLevel), log.WithOutput(out))
}

func logLines(logs *bytes.Buffer) []string {
	return strings.Split(strings.TrimSuffix(logs.String(), "\n"), "\n")
}

func loggerOutput(l log.Logger) io.Writer {
	var output io.Writer

	l.SetOptions(log.WithOutputWrapper(func(current io.Writer) io.Writer {
		output = current

		return current
	}))

	return output
}

var (
	csiPrefix = regexp.MustCompile(`^\x1b\[[0-9;?]*[@-~]`)
	csi       = regexp.MustCompile(`\x1b\[[0-9;?]*[@-~]`)
)

// drawnSegments returns the text between carriage returns in stream, without escapes and log lines.
func drawnSegments(stream []byte) []string {
	var segments []string

	for segment := range strings.SplitSeq(csi.ReplaceAllString(string(stream), ""), "\r") {
		if segment != "" && !strings.Contains(segment, "\n") {
			segments = append(segments, segment)
		}
	}

	return segments
}

func drawnLines(stream []byte) []string {
	var lines []string

	for _, segment := range drawnSegments(stream) {
		if line := strings.TrimRight(segment, " "); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// screenRows replays stream on a screen of width columns and returns the non-blank rows.
func screenRows(stream []byte, width int) []string {
	screen := newScreen(width)
	screen.write(stream)

	return screen.rows()
}

type screen struct {
	lines [][]rune
	line  int
	col   int
	width int
}

func newScreen(width int) *screen {
	return &screen{lines: [][]rune{{}}, width: width}
}

func (screen *screen) resize(width int) {
	screen.width = width
}

// write replays stream, handling \r, \n, erase down and printable characters.
func (screen *screen) write(stream []byte) {
	text := string(stream)

	for text != "" {
		if sequence := csiPrefix.FindString(text); sequence != "" {
			if strings.HasSuffix(sequence, "J") {
				screen.lines[screen.line] = screen.lines[screen.line][:min(screen.col, len(screen.lines[screen.line]))]
				screen.lines = screen.lines[:screen.line+1]
			}

			text = text[len(sequence):]

			continue
		}

		char, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		screen.put(char)
	}
}

func (screen *screen) put(char rune) {
	switch char {
	case '\r':
		if screen.col > 0 {
			screen.col = (screen.col - 1) / screen.width * screen.width
		}

		return
	case '\n':
		screen.line++
		screen.col = 0

		if screen.line == len(screen.lines) {
			screen.lines = append(screen.lines, []rune{})
		}

		return
	}

	columns := runewidth.RuneWidth(char)
	if columns == 0 {
		return
	}

	if screen.col%screen.width+columns > screen.width {
		screen.col += screen.width - screen.col%screen.width
	}

	cells := screen.lines[screen.line]
	for len(cells) < screen.col+columns {
		cells = append(cells, ' ')
	}

	cells[screen.col] = char
	for i := 1; i < columns; i++ {
		cells[screen.col+i] = 0
	}

	screen.lines[screen.line] = cells
	screen.col += columns
}

func (screen *screen) rows() []string {
	var shown []string

	for _, cells := range screen.lines {
		for start := 0; start < len(cells); start += screen.width {
			row := strings.ReplaceAll(string(cells[start:min(start+screen.width, len(cells))]), "\x00", "")
			if text := strings.TrimRight(row, " "); text != "" {
				shown = append(shown, text)
			}
		}
	}

	return shown
}
