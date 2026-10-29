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

// Messages holds the messages for an operation.
type Messages struct {
	// Working is shown while the operation runs (e.g. "Creating Git worktree for ref main...").
	Working string
	// Done is logged when the operation succeeds (e.g. "Created Git worktree for ref main").
	Done string
	// Started means the caller already logged Working, so it is not logged again.
	Started bool
}

// Options configures a [Reporter].
type Options struct {
	// Out is where the progress line is drawn.
	Out io.Writer
	// Width returns the terminal width of Out, or 0 if unknown.
	Width     func() int
	Env       map[string]string
	GOOS      string
	Threshold time.Duration
	// OutIsTTY is true when Out is a terminal that supports ANSI escape sequences.
	OutIsTTY bool
	// InIsTTY is true when stdin is a terminal.
	InIsTTY bool
	// LogsForHumans is true when logs are enabled, at info level or lower, and not JSON.
	LogsForHumans bool
}

// Reporter reports operations that run longer than its threshold.
type Reporter struct {
	term      *terminal
	threshold time.Duration
}

// TerminalOptions returns options for drawing on the stderr of v. Callers clear LogsForHumans for JSON logs.
func TerminalOptions(v *venv.Venv, l log.Logger) Options {
	v.RequireTerminal()
	v.RequireWriters()
	v.RequireGOOS()

	return Options{
		Out:           v.Writers.ErrWriter,
		Width:         v.Terminal.ErrWidth,
		Env:           v.Env,
		GOOS:          v.Platform.GOOS,
		Threshold:     DefaultThreshold,
		OutIsTTY:      v.Terminal.StderrIsTTY() && v.Terminal.StderrHandlesEscapes(),
		InIsTTY:       v.Terminal.StdinIsTTY(),
		LogsForHumans: !l.Formatter().DisabledOutput() && l.Level() >= log.InfoLevel,
	}
}

func New(opts Options) *Reporter {
	reporter := &Reporter{threshold: opts.Threshold}

	if animatable(&opts) {
		reporter.term = newTerminal(opts.Out, opts.Width, frames(opts.Env, opts.GOOS))
		reporter.term.echoesInput = opts.InIsTTY
	}

	return reporter
}

func ContextWithReporter(ctx context.Context, reporter *Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, reporter)
}

// ContextWithLogOnly returns a copy of ctx whose reporter only writes log lines.
func ContextWithLogOnly(ctx context.Context) context.Context {
	reporter := ReporterFromContext(ctx)
	if reporter == nil {
		return ctx
	}

	return ContextWithReporter(ctx, reporter.LogOnly())
}

// ReporterFromContext returns the reporter stored on ctx, or nil.
func ReporterFromContext(ctx context.Context) *Reporter {
	reporter, ok := ctx.Value(reporterKey{}).(*Reporter)
	if !ok {
		return nil
	}

	return reporter
}

// InputRead tells the reporter on ctx that a prompt was answered, so the progress line can be drawn again.
func InputRead(ctx context.Context) {
	reporter := ReporterFromContext(ctx)
	if reporter == nil || reporter.term == nil {
		return
	}

	reporter.term.inputRead()
}

// ShowAfter runs fn and reports it through the reporter on ctx, or only runs fn when there is none.
func ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	reporter := ReporterFromContext(ctx)
	if reporter == nil {
		return fn()
	}

	return reporter.ShowAfter(ctx, l, msgs, fn)
}

// Animated reports whether the reporter draws a progress line instead of log lines.
func (reporter *Reporter) Animated() bool {
	return reporter.term != nil
}

// LogOnly returns a reporter with the same threshold that only writes log lines.
func (reporter *Reporter) LogOnly() *Reporter {
	return &Reporter{threshold: reporter.threshold}
}

// Guard wraps w so each write clears and redraws the progress line. A w that logs would deadlock.
func (reporter *Reporter) Guard(w io.Writer) io.Writer {
	if reporter.term == nil || reporter.guards(w) {
		return w
	}

	return &guard{term: reporter.term, w: w}
}

// GuardLogger wraps the output of l with [Reporter.Guard] and returns a restore function.
func (reporter *Reporter) GuardLogger(l log.Logger) func() {
	if reporter.term == nil {
		return func() {
			// Nothing to restore, the logger was not wrapped.
		}
	}

	l.SetOptions(log.WithOutputWrapper(reporter.Guard))

	return func() {
		l.SetOptions(log.WithOutputWrapper(reporter.unguard))
	}
}

// ShowAfter runs fn, reports it after the threshold, and logs the done message if it succeeds.
func (reporter *Reporter) ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, reporter.threshold, msgs, fn)
}

// Show runs fn and reports it from the start, for operations known to be slow.
func (reporter *Reporter) Show(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, 0, msgs, fn)
}

func (reporter *Reporter) guards(w io.Writer) bool {
	guarded, ok := w.(*guard)

	return ok && guarded.term == reporter.term
}

// unguard returns the writer wrapped by w, or w if this reporter did not wrap it.
func (reporter *Reporter) unguard(w io.Writer) io.Writer {
	guarded, ok := w.(*guard)
	if !ok || guarded.term != reporter.term {
		return w
	}

	return guarded.w
}

// show runs fn, reports it after threshold, and logs the done message if it was reported and succeeded.
func (reporter *Reporter) show(
	ctx context.Context,
	l log.Logger,
	threshold time.Duration,
	msgs Messages,
	fn func() error,
) error {
	start := time.Now()
	finished := make(chan struct{})
	reported := make(chan bool, 1)

	go func() {
		reported <- reporter.watch(ctx, l, threshold, msgs, start, finished)
	}()

	shown := false

	err := func() error {
		// Also runs on panic, so the progress line is cleared first.
		defer func() {
			close(finished)

			shown = <-reported
		}()

		return fn()
	}()

	if shown && err == nil && ctx.Err() == nil {
		logDone(l, msgs.Done, start)
	}

	return err
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

	if reporter.term == nil {
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

// logProgress logs the working message, then repeats it every keepaliveInterval counted from start.
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
