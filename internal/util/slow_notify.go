package util

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/term"
)

// spinnerFrames are braille dot characters used for the progress spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 100 * time.Millisecond

// keepaliveInterval is how often a progress log line is emitted in non-interactive
// environments (no TTY) to prevent CI systems from killing the job due to inactivity.
const keepaliveInterval = 30 * time.Second

// spinnerLineOverhead is the number of extra bytes the spinner line uses beyond the message itself
// (braille character 3 bytes + space 1 byte). Since braille dots occupy a single terminal column,
// this over-clears by a couple of columns, which is harmless.
const spinnerLineOverhead = 4

// SlowNotifyMsg holds the messages for NotifyIfSlow.
type SlowNotifyMsg struct {
	// Spinner is shown while the operation is in progress (e.g. "Creating Git worktree for ref main...").
	Spinner string
	// Done is logged as INFO when the operation completes (e.g. "Created Git worktree for ref main").
	Done string
}

// SpinnerWriter returns os.Stderr if it is an interactive terminal, nil otherwise.
// Use the returned writer as the spinnerW argument to NotifyIfSlow.
func SpinnerWriter() io.Writer {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr
	}

	return nil
}

// NotifyIfSlow runs fn and, if it takes longer than timeout, shows a spinner on spinnerW.
// When fn completes successfully, the spinner is replaced by an INFO log with the done message and elapsed time.
// When fn returns an error, the spinner is cleared but no success message is logged.
func NotifyIfSlow(ctx context.Context, l log.Logger, spinnerW io.Writer, timeout time.Duration, msgs SlowNotifyMsg, fn func() error) error {
	result := make(chan error, 1)
	showed := make(chan struct{})
	start := time.Now()

	go notifyLoop(ctx, l, spinnerW, timeout, msgs, start, result, showed)

	err := fn()

	result <- err

	<-showed

	return err
}

// notifyLoop waits for the timeout, then shows a spinner until done.
// On successful completion it clears the spinner and logs the done message with elapsed time.
// On error or context cancellation no completion message is logged.
func notifyLoop(
	ctx context.Context,
	l log.Logger,
	spinnerW io.Writer,
	timeout time.Duration,
	msgs SlowNotifyMsg,
	start time.Time,
	result <-chan error,
	showed chan<- struct{},
) {
	defer close(showed)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-result:
		return
	case <-ctx.Done():
		return
	}

	// No spinner writer — log and emit periodic keepalive lines so CI systems
	// (e.g. CircleCI) do not kill the job due to prolonged output silence.
	if spinnerW == nil {
		l.Info(msgs.Spinner)

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				l.Infof("%s (%.0fs elapsed)", msgs.Spinner, time.Since(start).Seconds())
			case err := <-result:
				if err == nil {
					logDone(l, msgs.Done, start)
				}

				return
			case <-ctx.Done():
				return
			}
		}
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
		case err := <-result:
			clearSpinner(spinnerW, msgs.Spinner)

			if err == nil {
				logDone(l, msgs.Done, start)
			}

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
