package spinner_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/bits"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testThreshold = 50 * time.Millisecond

// dotsT and dotsG are the letters T and G as the animation draws them in braille dots.
const (
	dotsT = "\u28b9\u284f"
	dotsG = "\u288e\u28ed"
)

func TestShowAfterFastOperationIsNotReported(t *testing.T) {
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

func TestShowAfterSlowOperationLogsWorkingThenDone(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "completed",
		}, func() error {
			time.Sleep(200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 2)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
		assert.Contains(t, lines[1], `msg=completed`)
	})
}

func TestShowAfterFailureLogsNoDone(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		errBoom := errors.New("boom")

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "should not appear",
		}, func() error {
			time.Sleep(200 * time.Millisecond)

			return errBoom
		})

		require.ErrorIs(t, err, errBoom)

		lines := logLines(logs)
		require.Len(t, lines, 1)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
	})
}

func TestShowAfterDoneCarriesElapsedTime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			time.Sleep(1200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, logs.String(), `msg="finished (1.2s)"`)
	})
}

func TestShowAfterRepeatsWorkingLineAsKeepalive(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			time.Sleep(61 * time.Second)

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

func TestShowAfterStartedOperationIsNotAnnouncedTwice(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		err := logReporter().ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
			Started: true,
		}, func() error {
			time.Sleep(31 * time.Second)

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 2)
		assert.Contains(t, lines[0], `msg="working... (30s elapsed)"`)
		assert.Contains(t, lines[1], `msg="finished (31.0s)"`)
	})
}

func TestShowAfterCancelledBeforeThresholdIsNotReported(t *testing.T) {
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
			time.Sleep(100 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, logs.String())
	})
}

func TestShowAfterCancelledWhileReportedLogsNoDone(t *testing.T) {
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
			time.Sleep(150 * time.Millisecond)
			cancel()

			return nil
		})

		require.NoError(t, err)

		lines := logLines(logs)
		require.Len(t, lines, 1)
		assert.True(t, strings.HasSuffix(lines[0], "msg=working..."), lines[0])
	})
}

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

func TestShowAfterWithoutReporterOnlyRunsTheOperation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx := spinner.ContextWithLogOnly(t.Context())
		require.Nil(t, spinner.ReporterFromContext(ctx))

		ran := false

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			time.Sleep(time.Minute)

			ran = true

			return nil
		})

		require.NoError(t, err)
		assert.True(t, ran)
		assert.Empty(t, logs.String())
	})
}

func TestShowAfterReportsThroughTheContextReporter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		ctx := spinner.ContextWithReporter(t.Context(), logReporter())

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			time.Sleep(200 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)
		assert.Len(t, logLines(logs), 2)
	})
}

func TestContextWithLogOnlyNeverDraws(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		logs := new(bytes.Buffer)
		l := newLogger(logs)

		reporter := spinner.New(terminalOptions(term, 120))
		require.True(t, reporter.Animated())

		ctx := spinner.ContextWithLogOnly(spinner.ContextWithReporter(t.Context(), reporter))
		require.False(t, spinner.ReporterFromContext(ctx).Animated())

		err := spinner.ShowAfter(ctx, l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			time.Sleep(time.Second)

			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, term.String())
		assert.Len(t, logLines(logs), 2)
	})
}

func TestProgressLineShowsFrameMessageAndElapsedTime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := spinner.New(terminalOptions(term, 120))
		l := newLogger(reporter.Guard(term))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Cloning repository...",
			Done:    "Cloned repository",
		}, func() error {
			time.Sleep(2500 * time.Millisecond)

			return nil
		})

		require.NoError(t, err)

		frames := drawnLines(term.Bytes())
		require.NotEmpty(t, frames)
		assert.Equal(t, dotsT+" Cloning repository... (0s)", frames[0])
		assert.True(t, slices.ContainsFunc(frames, func(frame string) bool {
			return strings.HasSuffix(frame, " Cloning repository... (2s)")
		}), frames)

		screen := screenRows(term.Bytes(), 120)
		require.Len(t, screen, 1)
		assert.Contains(t, screen[0], `msg="Cloned repository (2.5s)"`)
	})
}

func TestProgressLineNeverWraps(t *testing.T) {
	t.Parallel()

	const (
		width = 80
		url   = "git::https://github.com/gruntwork-io/terraform-aws-service-catalog.git?ref=v0.118.4"
	)

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := spinner.New(terminalOptions(term, width))
		l := newLogger(reporter.Guard(term))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Downloading source from " + url + "...",
			Done:    "Downloaded",
		}, func() error {
			time.Sleep(3 * time.Second)

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

func TestProgressLineStaysOnOneRowWhateverTheMessageHolds(t *testing.T) {
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
			want:    dotsT + " Reading a b c  d (0s)",
		},
		{
			name:    "wide characters are counted as two columns",
			working: "Downloading " + strings.Repeat("\uff21\uff22", 10),
			want:    dotsT + " Downloading " + strings.Repeat("\uff21\uff22", 4) + "... (0s)",
		},
		{
			name:    "combining marks take no column",
			working: "Cloning cafe\u0301 repo 0123456789 0123456789 0123456789",
			want:    dotsT + " Cloning cafe\u0301 repo 0123456789... (0s)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				term := new(syncBuffer)
				reporter := spinner.New(terminalOptions(term, width))
				l := newLogger(reporter.Guard(term))

				err := reporter.Show(t.Context(), l, spinner.Messages{
					Working: tc.working,
					Done:    "done",
				}, func() error {
					time.Sleep(50 * time.Millisecond)

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

func TestProgressLineIsSharedAndLogsAreWrittenAroundIt(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := spinner.New(terminalOptions(term, 200))
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

		time.Sleep(1550 * time.Millisecond)
		l.Info("unit-c: tofu init finished")

		during := screenRows(term.Bytes(), 200)

		wg.Wait()

		require.NoError(t, errs[0])
		require.NoError(t, errs[1])

		require.Len(t, during, 2)
		assert.True(t, strings.HasPrefix(during[0], "time="), during[0])
		assert.Contains(t, during[0], `msg="unit-c: tofu init finished"`)
		assert.Equal(t, dotsG+" Downloading source... (1s) +1 more", during[1])

		after := screenRows(term.Bytes(), 200)
		require.Len(t, after, 3)

		for _, row := range after {
			assert.True(t, strings.HasPrefix(row, "time="), row)
			assert.NotContains(t, row, "Downloading source...")
		}
	})
}

func TestProgressLineIsClearedWhenTheOperationPanics(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := spinner.New(terminalOptions(term, 120))
		l := newLogger(reporter.Guard(term))

		var err error

		require.Panics(t, func() {
			err = reporter.ShowAfter(t.Context(), l, spinner.Messages{
				Working: "working...",
				Done:    "done",
			}, func() error {
				time.Sleep(200 * time.Millisecond)
				panic("boom")
			})
		})
		require.NoError(t, err)

		drawn := len(term.Bytes())
		require.NotZero(t, drawn)
		assert.Empty(t, screenRows(term.Bytes(), 120))

		time.Sleep(5 * time.Second)

		assert.Len(t, term.Bytes(), drawn)
	})
}

func TestProgressFramesFollowTheTerminal(t *testing.T) {
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
			wantFrame: dotsT,
		},
		{
			name:      "linux with the C locale",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "C"},
			wantFrame: "T",
		},
		{
			name:      "linux with no locale",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm"},
			wantFrame: "T",
		},
		{
			name:      "LC_ALL wins over LANG",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "en_US.UTF-8", "LC_ALL": "POSIX"},
			wantFrame: "T",
		},
		{
			name:      "LC_CTYPE wins over LANG",
			goos:      "linux",
			env:       map[string]string{"TERM": "xterm", "LANG": "C", "LC_CTYPE": "en_US.utf8"},
			wantFrame: dotsT,
		},
		{
			name:      "macOS terminal",
			goos:      "darwin",
			env:       map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"},
			wantFrame: dotsT,
		},
		{
			name:      "windows console",
			goos:      "windows",
			env:       map[string]string{},
			wantFrame: "T",
		},
		{
			name:      "windows console ignores the locale",
			goos:      "windows",
			env:       map[string]string{"LANG": "en_US.UTF-8"},
			wantFrame: "T",
		},
		{
			name:      "windows terminal",
			goos:      "windows",
			env:       map[string]string{"WT_SESSION": "c4a9a1b2"},
			wantFrame: dotsT,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				term := new(syncBuffer)

				opts := terminalOptions(term, 120)
				opts.Env = tc.env
				opts.GOOS = tc.goos

				reporter := spinner.New(opts)
				require.True(t, reporter.Animated())

				l := newLogger(reporter.Guard(term))

				err := reporter.Show(t.Context(), l, spinner.Messages{
					Working: "working",
					Done:    "done",
				}, func() error {
					time.Sleep(50 * time.Millisecond)

					return nil
				})

				require.NoError(t, err)

				frames := drawnLines(term.Bytes())
				require.NotEmpty(t, frames)
				assert.Equal(t, tc.wantFrame+" working (0s)", frames[0])
			})
		})
	}
}

func TestProgressLettersChangeOneDotAtATime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		reporter := spinner.New(terminalOptions(term, 120))
		l := newLogger(reporter.Guard(term))

		err := reporter.Show(t.Context(), l, spinner.Messages{
			Working: "working",
			Done:    "done",
		}, func() error {
			time.Sleep(3 * time.Second)

			return nil
		})

		require.NoError(t, err)

		var letters []string

		for _, line := range drawnLines(term.Bytes()) {
			letter, _, found := strings.Cut(line, " ")
			require.True(t, found, line)

			if len(letters) == 0 || letters[len(letters)-1] != letter {
				letters = append(letters, letter)
			}
		}

		require.GreaterOrEqual(t, len(letters), 17, letters)
		assert.Equal(t, dotsT, letters[0])
		assert.Equal(t, dotsG, letters[8])
		assert.Equal(t, dotsT, letters[16])

		for i := range len(letters) - 1 {
			assert.Equal(t, 1, changedDots(letters[i], letters[i+1]), "%s -> %s", letters[i], letters[i+1])
		}
	})
}

func TestProgressLettersTakeTurnsWithoutBraille(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)

		opts := terminalOptions(term, 120)
		opts.Env = map[string]string{"TERM": "xterm", "LANG": "C"}

		reporter := spinner.New(opts)
		l := newLogger(reporter.Guard(term))

		err := reporter.Show(t.Context(), l, spinner.Messages{
			Working: "working",
			Done:    "done",
		}, func() error {
			time.Sleep(3 * time.Second)

			return nil
		})

		require.NoError(t, err)

		var letters []string

		for _, line := range drawnLines(term.Bytes()) {
			letter, _, found := strings.Cut(line, " ")
			require.True(t, found, line)

			if len(letters) == 0 || letters[len(letters)-1] != letter {
				letters = append(letters, letter)
			}
		}

		assert.Equal(t, []string{"T", "G", "T"}, letters)
	})
}

func TestProgressWriteFailureIsLoggedOnceAndTheOperationCompletes(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logs := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.DebugLevel), log.WithOutput(logs))

		reporter := spinner.New(terminalOptions(failingWriter{}, 120))
		require.True(t, reporter.Animated())

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "finished",
		}, func() error {
			time.Sleep(2 * time.Second)

			return nil
		})

		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(logs.String(), "Progress is no longer drawn on the terminal"))
		assert.Contains(t, logs.String(), "terminal is gone")
		assert.Contains(t, logs.String(), `msg="finished (2.0s)"`)
	})
}

func TestProgressLineIsNotDrawnOnATerminalThatGotTooNarrow(t *testing.T) {
	t.Parallel()

	const narrow = 10

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		start := time.Now()

		opts := terminalOptions(term, 120)
		opts.Width = func() int {
			if time.Since(start) < time.Second {
				return 120
			}

			return narrow
		}

		reporter := spinner.New(opts)
		l := newLogger(new(bytes.Buffer))

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "working...",
			Done:    "done",
		}, func() error {
			time.Sleep(500 * time.Millisecond)

			require.Len(t, screenRows(term.Bytes(), 120), 1)

			time.Sleep(510 * time.Millisecond)

			before := len(term.Bytes())

			time.Sleep(2 * time.Second)

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

func TestProgressLineWaitsForAPromptToBeAnswered(t *testing.T) {
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

				opts := terminalOptions(term, 120)
				opts.InIsTTY = tc.inputIsTTY

				reporter := spinner.New(opts)
				guarded := reporter.Guard(term)
				l := newLogger(guarded)
				ctx := spinner.ContextWithReporter(t.Context(), reporter)

				err := spinner.ShowAfter(ctx, l, spinner.Messages{
					Working: "working...",
					Done:    "done",
				}, func() error {
					time.Sleep(300 * time.Millisecond)

					_, err := guarded.Write([]byte("Create the bucket? (y/n) "))
					require.NoError(t, err)

					time.Sleep(time.Second)

					assert.Equal(t, []string{"Create the bucket? (y/n)"}, screenRows(term.Bytes(), 120))

					if tc.inputIsTTY {
						// A terminal echoes the answer and its line break itself.
						_, err = term.Write([]byte("y\n"))
						require.NoError(t, err)
					}

					spinner.InputRead(ctx)

					before := len(term.Bytes())

					time.Sleep(time.Second)

					assert.Equal(t, tc.wantDrawAfter, len(drawnLines(term.Bytes()[before:])) > 0)

					return nil
				})

				require.NoError(t, err)
			})
		})
	}
}

func TestGuardOfALogReporterIsTheWriterItself(t *testing.T) {
	t.Parallel()

	w := new(bytes.Buffer)

	assert.Same(t, w, logReporter().Guard(w))
}

func TestTerminalOptionsDescribeTheRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		change            func(l log.Logger)
		name              string
		wantLogsForHumans bool
	}{
		{
			name:              "info logs",
			wantLogsForHumans: true,
		},
		{
			name:              "debug logs",
			change:            func(l log.Logger) { l.SetOptions(log.WithLevel(log.DebugLevel)) },
			wantLogsForHumans: true,
		},
		{
			name:   "warn logs",
			change: func(l log.Logger) { l.SetOptions(log.WithLevel(log.WarnLevel)) },
		},
		{
			name:   "logs disabled",
			change: func(l log.Logger) { l.Formatter().SetDisabledOutput(true) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errWriter := new(bytes.Buffer)
			env := map[string]string{"TERM": "xterm"}

			v := &venv.Venv{
				Env:      env,
				Platform: &venv.Platform{GOOS: "linux"},
				Terminal: &venv.Terminal{
					StdinIsTTY:  func() bool { return true },
					StderrIsTTY: func() bool { return true },
					ErrWidth:    func() int { return 132 },
				},
				Writers: &writer.Writers{Writer: new(bytes.Buffer), ErrWriter: errWriter},
			}

			l := log.New(
				log.WithLevel(log.InfoLevel),
				log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
			)

			if tc.change != nil {
				tc.change(l)
			}

			opts := spinner.TerminalOptions(v, l)

			assert.Same(t, errWriter, opts.Out)
			assert.Equal(t, 132, opts.Width())
			assert.Equal(t, env, opts.Env)
			assert.Equal(t, "linux", opts.GOOS)
			assert.Equal(t, spinner.DefaultThreshold, opts.Threshold)
			assert.True(t, opts.OutIsTTY)
			assert.True(t, opts.InIsTTY)
			assert.Equal(t, tc.wantLogsForHumans, opts.LogsForHumans)
		})
	}
}

func TestGuardIsNotStacked(t *testing.T) {
	t.Parallel()

	reporter := spinner.New(terminalOptions(new(syncBuffer), 120))
	guarded := reporter.Guard(new(bytes.Buffer))

	assert.Same(t, guarded, reporter.Guard(guarded))
}

func TestGuardLoggerIsUndone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reporter    *spinner.Reporter
		name        string
		wantGuarded bool
	}{
		{
			name:        "animated reporter",
			reporter:    spinner.New(terminalOptions(new(syncBuffer), 120)),
			wantGuarded: true,
		},
		{
			name:     "log reporter",
			reporter: logReporter(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			output := new(bytes.Buffer)
			l := newLogger(output)

			restore := tc.reporter.GuardLogger(l)

			assert.Equal(t, tc.wantGuarded, loggerOutput(l) != io.Writer(output))

			restore()

			assert.Same(t, output, loggerOutput(l))
		})
	}
}

func TestProgressLineDoesNotWrapWhenTheTerminalShrinks(t *testing.T) {
	t.Parallel()

	const narrow = 30

	synctest.Test(t, func(t *testing.T) {
		term := new(syncBuffer)
		start := time.Now()

		opts := terminalOptions(term, 120)
		opts.Width = func() int {
			if time.Since(start) < time.Second {
				return 120
			}

			return narrow
		}

		reporter := spinner.New(opts)
		l := newLogger(reporter.Guard(term))

		var before int

		err := reporter.ShowAfter(t.Context(), l, spinner.Messages{
			Working: "Downloading source from git::https://github.com/acme/modules.git?ref=v1.2.3...",
			Done:    "done",
		}, func() error {
			time.Sleep(1010 * time.Millisecond)

			before = len(term.Bytes())

			time.Sleep(2 * time.Second)

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

// syncBuffer is a buffer safe for the concurrent writes a terminal receives.
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

// failingWriter is a terminal that went away.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("terminal is gone")
}

// logReporter returns a reporter that writes log lines.
func logReporter() *spinner.Reporter {
	return spinner.New(spinner.Options{Threshold: testThreshold})
}

// terminalOptions returns the options of a UTF-8 terminal `width` columns wide that draws on `out`.
func terminalOptions(out interface{ Write([]byte) (int, error) }, width int) spinner.Options {
	return spinner.Options{
		Out:           out,
		Width:         func() int { return width },
		Env:           map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"},
		GOOS:          "linux",
		Threshold:     testThreshold,
		OutIsTTY:      true,
		LogsForHumans: true,
	}
}

// newLogger returns a logger that writes info lines to `out`.
func newLogger(out interface{ Write([]byte) (int, error) }) log.Logger {
	return log.New(log.WithLevel(log.InfoLevel), log.WithOutput(out))
}

// logLines returns the lines written to `logs`.
func logLines(logs *bytes.Buffer) []string {
	return strings.Split(strings.TrimSuffix(logs.String(), "\n"), "\n")
}

// changedDots returns how many braille dots differ between `a` and `b`, two letters of equal length.
func changedDots(a, b string) int {
	changed := 0

	other := []rune(b)

	for i, char := range []rune(a) {
		changed += bits.OnesCount32(uint32(char ^ other[i]))
	}

	return changed
}

// loggerOutput returns the writer `l` writes to.
func loggerOutput(l log.Logger) io.Writer {
	var output io.Writer

	l.SetOptions(log.WithOutputWrapper(func(current io.Writer) io.Writer {
		output = current

		return current
	}))

	return output
}

// drawnSegments returns what was written to `stream` between carriage returns, log lines left out.
func drawnSegments(stream []byte) []string {
	var segments []string

	for segment := range strings.SplitSeq(string(stream), "\r") {
		if segment != "" && !strings.Contains(segment, "\n") {
			segments = append(segments, segment)
		}
	}

	return segments
}

// drawnLines returns every progress line drawn in `stream`, in order.
func drawnLines(stream []byte) []string {
	var lines []string

	for _, segment := range drawnSegments(stream) {
		if line := strings.TrimRight(segment, " "); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// screenRows replays `stream` on a terminal `width` columns wide and returns the rows that are not blank.
func screenRows(stream []byte, width int) []string {
	rows := [][]rune{{}}
	row, col := 0, 0

	for _, char := range string(stream) {
		switch char {
		case '\r':
			col = 0

			continue
		case '\n':
			row++
			col = 0

			rows = append(rows, []rune{})

			continue
		}

		columns := runewidth.RuneWidth(char)
		if columns == 0 {
			continue
		}

		if col+columns > width {
			row++
			col = 0

			rows = append(rows, []rune{})
		}

		for len(rows[row]) <= col {
			rows[row] = append(rows[row], ' ')
		}

		rows[row][col] = char
		col += columns
	}

	shown := make([]string, 0, len(rows))

	for _, cells := range rows {
		if text := strings.TrimRight(string(cells), " "); text != "" {
			shown = append(shown, text)
		}
	}

	return shown
}
