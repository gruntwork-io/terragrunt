// Package spinner reports the progress of operations the user is waiting on.
//
// A [Reporter] is built once for a run. On a terminal it animates one progress
// line that every operation in flight shares. Anywhere else it writes log
// lines, repeated with an elapsed time so a CI system watching for output does
// not take the wait for a hang.
//
// A run carries its reporter on the context. [ShowAfter] reports through that
// reporter, and only runs the operation when the context carries none.
package spinner

import (
	"context"
	"io"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DefaultThreshold is how long an operation runs before it is reported.
const DefaultThreshold = time.Second

// keepaliveInterval is how often a log line repeats while an operation runs
// with no progress line, so a CI system does not end the job for being silent.
const keepaliveInterval = 30 * time.Second

// reporterKey is the context key a run's [Reporter] is stored under.
type reporterKey struct{}

// Messages are the texts an operation is reported with.
type Messages struct {
	// Working is shown while the operation runs, e.g. "Creating Git worktree for reference main...".
	Working string
	// Done is logged once the operation succeeds, e.g. "Created Git worktree for reference main".
	Done string
	// Started says the caller already logged that the operation started, so `Working` is not logged a second time.
	Started bool
}

// Options describe the terminal and the run a [Reporter] reports for.
type Options struct {
	// Out is the stream the progress line is drawn on.
	Out io.Writer
	// Width reports how many columns the terminal behind `Out` has, or 0 when that is not known.
	Width func() int
	// Env is the environment of the run.
	Env map[string]string
	// GOOS is the operating system the run is on.
	GOOS string
	// Threshold is how long an operation runs before it is reported.
	Threshold time.Duration
	// OutIsTTY says that `Out` is a terminal.
	OutIsTTY bool
	// InIsTTY says that the input of the run is a terminal, which echoes what the user types.
	InIsTTY bool
	// LogsForHumans says that log output is on, shows the info level, and is not a format a program reads.
	LogsForHumans bool
}

// Reporter reports the operations of a run that take longer than its threshold.
type Reporter struct {
	term      *terminal
	threshold time.Duration
}

// TerminalOptions returns the options for a run that logs with `l` and whose
// error stream, the one of `v`, is where progress is drawn. `LogsForHumans`
// covers what `l` knows: the caller clears it for a log format a program reads.
// `l` must have a formatter.
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
		OutIsTTY:      v.Terminal.StderrIsTTY(),
		InIsTTY:       v.Terminal.StdinIsTTY(),
		LogsForHumans: !l.Formatter().DisabledOutput() && l.Level() >= log.InfoLevel,
	}
}

// New returns a reporter for the run `opts` describe.
func New(opts Options) *Reporter {
	reporter := &Reporter{threshold: opts.Threshold}

	if animatable(&opts) {
		reporter.term = newTerminal(opts.Out, opts.Width, frames(opts.Env, opts.GOOS))
		reporter.term.echoesInput = opts.InIsTTY
	}

	return reporter
}

// ContextWithReporter returns a copy of `ctx` that carries `reporter`.
func ContextWithReporter(ctx context.Context, reporter *Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, reporter)
}

// ContextWithLogOnly returns a copy of `ctx` whose reporter writes log lines
// and never draws a progress line. Use it for work whose output reaches the
// terminal while other operations are reported, such as units running
// concurrently. A `ctx` with no reporter is returned as it is.
func ContextWithLogOnly(ctx context.Context) context.Context {
	reporter := ReporterFromContext(ctx)
	if reporter == nil {
		return ctx
	}

	return ContextWithReporter(ctx, reporter.LogOnly())
}

// ReporterFromContext returns the reporter `ctx` carries, or nil when it carries none.
func ReporterFromContext(ctx context.Context) *Reporter {
	reporter, ok := ctx.Value(reporterKey{}).(*Reporter)
	if !ok {
		return nil
	}

	return reporter
}

// InputRead tells the reporter `ctx` carries that the user answered a prompt.
// On a terminal the answer ends with a line break the terminal echoes itself,
// so the cursor is at the start of a row again and the progress line can be
// drawn. With no reporter, or one that draws nothing, it does nothing.
func InputRead(ctx context.Context) {
	reporter := ReporterFromContext(ctx)
	if reporter == nil || reporter.term == nil {
		return
	}

	reporter.term.inputRead()
}

// ShowAfter runs `fn` and reports it through the reporter `ctx` carries once it
// has run for the reporter's threshold. With no reporter it only runs `fn`.
func ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	reporter := ReporterFromContext(ctx)
	if reporter == nil {
		return fn()
	}

	return reporter.ShowAfter(ctx, l, msgs, fn)
}

// Animated reports whether `reporter` draws a progress line rather than writing log lines.
func (reporter *Reporter) Animated() bool {
	return reporter.term != nil
}

// LogOnly returns a reporter with the threshold of `reporter` that writes log lines.
func (reporter *Reporter) LogOnly() *Reporter {
	return &Reporter{threshold: reporter.threshold}
}

// Guard returns a writer to `w` that takes the progress line off the screen
// before each write and puts it back after, so the two never share a row. A
// reporter that is not animated returns `w` itself. `w` must reach the
// terminal directly: a `w` that logs, or that writes through another guard of
// `reporter`, would wait on the lock its own write holds.
func (reporter *Reporter) Guard(w io.Writer) io.Writer {
	if reporter.term == nil || reporter.guards(w) {
		return w
	}

	return &guard{term: reporter.term, w: w}
}

// GuardLogger routes the output of `l` through [Reporter.Guard] and returns
// the function that undoes it. A reporter that is not animated changes nothing.
func (reporter *Reporter) GuardLogger(l log.Logger) func() {
	if reporter.term == nil {
		return func() {}
	}

	l.SetOptions(log.WithOutputWrapper(reporter.Guard))

	return func() {
		l.SetOptions(log.WithOutputWrapper(reporter.unguard))
	}
}

// ShowAfter runs `fn` and reports it once it has run for the reporter's
// threshold, so an operation that is usually quick stays unreported. When `fn`
// succeeds after it was reported, the done message is logged with the time it
// took. When `fn` fails or `ctx` ends first, no done message is logged. The
// error of `fn` is returned as it is.
func (reporter *Reporter) ShowAfter(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, reporter.threshold, msgs, fn)
}

// Show runs `fn` and reports it from the moment it starts, for an operation
// already known to be slow.
func (reporter *Reporter) Show(ctx context.Context, l log.Logger, msgs Messages, fn func() error) error {
	return reporter.show(ctx, l, 0, msgs, fn)
}

// guards reports whether `w` is already a guard for the progress line of `reporter`.
func (reporter *Reporter) guards(w io.Writer) bool {
	guarded, ok := w.(*guard)

	return ok && guarded.term == reporter.term
}

// unguard returns the writer that `w` guards for `reporter`, and any other writer as it is.
func (reporter *Reporter) unguard(w io.Writer) io.Writer {
	guarded, ok := w.(*guard)
	if !ok || guarded.term != reporter.term {
		return w
	}

	return guarded.w
}

// show runs `fn`, has it watched from `threshold` on, and logs the done message
// when a reported `fn` succeeds before `ctx` ends.
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
		// Runs when `fn` panics too, so the progress line is gone before the panic reaches the caller.
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

// watch waits for `threshold`, then reports the operation until it finishes or
// `ctx` ends. It returns whether the operation was reported.
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

// waitTick blocks until `tick` fires and returns true. It returns false as
// soon as the operation finishes or `ctx` ends, and those win over a tick that
// fires at the same moment.
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

// logProgress logs the working message, then repeats it with the elapsed time
// until the operation finishes or `ctx` ends.
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

	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for waitTick(ctx, ticker.C, finished) {
		l.Infof("%s (%.0fs elapsed)", msgs.Working, time.Since(start).Seconds())
	}
}

// logDone logs the done message, with the elapsed time once it reaches a second.
func logDone(l log.Logger, msg string, start time.Time) {
	elapsed := time.Since(start)
	if elapsed < time.Second {
		l.Info(msg)

		return
	}

	l.Infof("%s (%.1fs)", msg, elapsed.Seconds())
}
