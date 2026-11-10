// Package spinner reports slow operations as a progress line on a terminal and as log lines elsewhere.
package spinner

import (
	"context"
	"io"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// DefaultThreshold is how long an operation runs before it is reported.
	DefaultThreshold = 100 * time.Millisecond

	keepaliveInterval = 30 * time.Second
)

type reporterKey struct{}

// Messages describes how one operation is reported.
type Messages struct {
	Working string
	Done    string
	Started bool
}

// Options configures [New].
type Options struct {
	Threshold     time.Duration
	LogsForHumans bool
}

// Reporter reports slow operations as a progress line or as log lines.
type Reporter struct {
	term      *terminal
	threshold time.Duration
	logs      bool
}

// New returns a reporter that draws on the stderr of v when it is a terminal, and writes log lines otherwise.
func New(v *venv.Venv, opts Options) *Reporter {
	v.RequireTerminal()
	v.RequireWriters()
	v.RequireGOOS()

	if !animatable(v, opts) {
		return &Reporter{threshold: opts.Threshold, logs: true}
	}

	term := newTerminal(v.Writers.ErrWriter, v.Terminal.ErrWidth, frames(v.Env, v.Platform.GOOS))
	term.echoesInput = v.Terminal.StdinIsTTY()

	return &Reporter{term: term, threshold: opts.Threshold}
}

// ContextWithReporter returns ctx with reporter installed for [ShowAfter] and [InputRead].
func ContextWithReporter(ctx context.Context, reporter *Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, reporter)
}

// ContextWithLogOnly returns ctx with a reporter that writes log lines where the reporter on ctx draws.
func ContextWithLogOnly(ctx context.Context) context.Context {
	reporter := reporterFromContext(ctx)

	return ContextWithReporter(ctx, &Reporter{threshold: reporter.threshold, logs: reporter.logs || reporter.Animated()})
}

// InputRead tells the reporter on ctx that a prompt was answered.
func InputRead(ctx context.Context) {
	reporterFromContext(ctx).inputRead()
}

// ShowAfter runs fn and reports it through the reporter on ctx.
func ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporterFromContext(ctx).ShowAfter(ctx, l, msgs, fn)
}

// Animated reports whether the reporter draws a progress line.
func (reporter *Reporter) Animated() bool {
	return reporter.term != nil
}

// Guard wraps w so each write clears and redraws the progress line of an animated reporter.
func (reporter *Reporter) Guard(w io.Writer) io.Writer {
	return &guard{term: reporter.term, w: w}
}

// GuardLogger wraps the output of l with [Reporter.Guard] and returns a function that puts the output back.
func (reporter *Reporter) GuardLogger(l log.Logger) func() {
	var output io.Writer

	l.SetOptions(log.WithOutputWrapper(func(w io.Writer) io.Writer {
		output = w

		return reporter.Guard(w)
	}))

	return func() {
		l.SetOptions(log.WithOutput(output))
	}
}

// ShowAfter runs fn, reports it past the threshold, and logs the done message if it was reported and succeeded.
func (reporter *Reporter) ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, reporter.threshold, msgs, fn)
}

// Show runs fn and reports it from the start, for operations known to be slow.
func (reporter *Reporter) Show(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, 0, msgs, fn)
}

func (reporter *Reporter) inputRead() {
	if !reporter.Animated() {
		return
	}

	reporter.term.inputRead()
}

func (reporter *Reporter) show(
	ctx context.Context,
	l log.Logger,
	threshold time.Duration,
	msgs Messages,
	fn func() error,
) error {
	if !reporter.Animated() && !reporter.logs {
		return fn()
	}

	start := time.Now()
	finished := make(chan struct{})
	reported := make(chan bool, 1)

	go func() {
		reported <- reporter.watch(ctx, l, threshold, msgs, start, finished)
	}()

	shown := false

	err := func() error {
		defer func() {
			close(finished)

			shown = <-reported
		}()

		return fn()
	}()

	if err != nil || !shown {
		return err
	}

	if ctx.Err() == nil {
		logDone(l, msgs.Done, start)
	}

	return nil
}

// watch reports the operation from threshold until it finishes and returns whether it was reported.
func (reporter *Reporter) watch(
	ctx context.Context,
	l log.Logger,
	threshold time.Duration,
	msgs Messages,
	start time.Time,
	finished <-chan struct{},
) bool {
	if threshold > 0 {
		timer := time.NewTimer(threshold)
		defer timer.Stop()

		if !waitTick(ctx, timer.C, finished) {
			return false
		}
	}

	if !reporter.Animated() {
		logProgress(ctx, l, msgs, start, finished)

		return true
	}

	reporter.term.animate(ctx, l, msgs.Working, start, finished)

	return true
}

// waitTick returns true when tick fires first, false when the operation finishes or ctx is cancelled.
func waitTick(ctx context.Context, tick <-chan time.Time, finished <-chan struct{}) bool {
	select {
	case <-tick:
	case <-finished:
		return false
	case <-ctx.Done():
		return false
	}

	select {
	case <-finished:
		return false
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

// logProgress logs the working message unless the caller did, then repeats it every keepaliveInterval from start.
func logProgress(
	ctx context.Context,
	l log.Logger,
	msgs Messages,
	start time.Time,
	finished <-chan struct{},
) {
	if !msgs.Started {
		l.Info(msgs.Working)
	}

	next := start.Add(keepaliveInterval)

	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	for waitTick(ctx, timer.C, finished) {
		l.Infof("%s (%.0fs elapsed)", msgs.Working, time.Since(start).Seconds())

		next = next.Add(keepaliveInterval)
		timer.Reset(time.Until(next))
	}
}

// logDone logs the done message, with the elapsed time if it is 1s or more.
func logDone(l log.Logger, msg string, start time.Time) {
	elapsed := time.Since(start)
	if elapsed < time.Second {
		l.Info(msg)

		return
	}

	l.Infof("%s (%.1fs)", msg, elapsed.Seconds())
}

// reporterFromContext returns the reporter on ctx, or a reporter that reports nothing when ctx has none.
func reporterFromContext(ctx context.Context) *Reporter {
	reporter, ok := ctx.Value(reporterKey{}).(*Reporter)
	if !ok {
		return new(Reporter)
	}

	return reporter
}
