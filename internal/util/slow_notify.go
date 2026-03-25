package util

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// spinnerFrames are braille dot characters used for the progress spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 200 * time.Millisecond

// spinnerLineOverhead is the number of extra characters the spinner line uses beyond the message itself
// (carriage return prefix, spinner frame, space separator, trailing padding).
const spinnerLineOverhead = 4

// SlowNotifyMsg holds the messages for NotifyIfSlow.
type SlowNotifyMsg struct {
	// Spinner is shown while the operation is in progress (e.g. "Creating Git worktree for ref main...").
	Spinner string
	// Done is logged as INFO when the operation completes (e.g. "Created Git worktree for ref main").
	Done string
}

// NotifyIfSlow runs fn and, if it takes longer than timeout, shows a spinner on spinnerW.
// When fn completes, the spinner is replaced by an INFO log with the done message and elapsed time.
func NotifyIfSlow(ctx context.Context, l log.Logger, spinnerW io.Writer, timeout time.Duration, msgs SlowNotifyMsg, fn func() error) error {
	done := make(chan struct{})
	showed := make(chan struct{})
	start := time.Now()

	go notifyLoop(ctx, l, spinnerW, timeout, msgs, start, done, showed)

	err := fn()

	close(done)
	<-showed

	return err
}

// NotifyIfSlowV is the generic variant of NotifyIfSlow that returns a value alongside the error.
func NotifyIfSlowV[T any](ctx context.Context, l log.Logger, spinnerW io.Writer, timeout time.Duration, msgs SlowNotifyMsg, fn func() (T, error)) (T, error) {
	done := make(chan struct{})
	showed := make(chan struct{})
	start := time.Now()

	go notifyLoop(ctx, l, spinnerW, timeout, msgs, start, done, showed)

	val, err := fn()

	close(done)
	<-showed

	return val, err
}

// notifyLoop waits for the timeout, then shows a spinner until done.
// On completion it clears the spinner and logs the done message with elapsed time.
func notifyLoop(
	ctx context.Context,
	l log.Logger,
	spinnerW io.Writer,
	timeout time.Duration,
	msgs SlowNotifyMsg,
	start time.Time,
	done <-chan struct{},
	showed chan<- struct{},
) {
	defer close(showed)

	select {
	case <-time.After(timeout):
	case <-done:
		return
	case <-ctx.Done():
		return
	}

	// No spinner writer — just log and wait.
	if spinnerW == nil {
		l.Info(msgs.Spinner)

		select {
		case <-done:
		case <-ctx.Done():
		}

		logDone(l, msgs.Done, start)

		return
	}

	// Animate spinner until the operation finishes.
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	frame := 0

	writeSpinnerFrame(spinnerW, spinnerFrames[0], msgs.Spinner)

	frame++

	for {
		select {
		case <-ticker.C:
			writeSpinnerFrame(spinnerW, spinnerFrames[frame%len(spinnerFrames)], msgs.Spinner)

			frame++
		case <-done:
			clearSpinner(spinnerW, msgs.Spinner)
			logDone(l, msgs.Done, start)

			return
		case <-ctx.Done():
			clearSpinner(spinnerW, msgs.Spinner)

			return
		}
	}
}

// logDone logs the completion message, appending elapsed seconds when > 1s.
func logDone(l log.Logger, msg string, start time.Time) {
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		l.Infof("%s (%.1fs)", msg, elapsed.Seconds())
	} else {
		l.Info(msg)
	}
}

// writeSpinnerFrame writes a single spinner frame to the writer.
func writeSpinnerFrame(w io.Writer, frame, msg string) {
	_, _ = fmt.Fprintf(w, "\r%s %s", frame, msg)
}

// clearSpinner overwrites the spinner line with spaces and returns the cursor to the start.
func clearSpinner(w io.Writer, msg string) {
	blank := strings.Repeat(" ", len(msg)+spinnerLineOverhead)

	_, _ = fmt.Fprintf(w, "\r%s\r", blank)
}
