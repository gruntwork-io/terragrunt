package spinner

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// frameInterval is how often the progress line is redrawn.
const frameInterval = 100 * time.Millisecond

// ellipsis ends a message that was cut to fit the terminal.
const ellipsis = "..."

// minMessageWidth is the fewest columns a message is cut down to. With less
// room the line shows the frame and the elapsed time alone.
const minMessageWidth = len(ellipsis) + 1

// terminal is the one progress line of a run. Every operation being reported
// shares it, and so does every [guard] writing around it.
type terminal struct {
	epoch         time.Time
	out           io.Writer
	failure       error
	width         func() int
	last          string
	frames        []string
	ops           []*operation
	drawn         int
	mu            sync.Mutex
	midLine       bool
	failureLogged bool
	echoesInput   bool
}

// operation is one operation on the progress line.
type operation struct {
	start time.Time
	text  string
}

// guard writes to `w` around the progress line of `term`.
type guard struct {
	term *terminal
	w    io.Writer
}

// Write takes the progress line off the screen, writes `p`, and draws the line
// again once the cursor is back at the start of a row.
func (guard *guard) Write(p []byte) (int, error) {
	guard.term.mu.Lock()
	defer guard.term.mu.Unlock()

	guard.term.clear()

	n, err := guard.w.Write(p)

	if len(p) > 0 {
		guard.term.midLine = p[len(p)-1] != '\n'
	}

	guard.term.draw(time.Now())

	return n, err
}

// newTerminal returns the progress line drawn on `out` with `frames`.
func newTerminal(out io.Writer, width func() int, frames []string) *terminal {
	return &terminal{
		epoch:  time.Now(),
		out:    out,
		width:  width,
		frames: frames,
	}
}

// animate keeps `text` on the progress line until the operation finishes or `ctx` ends.
func (term *terminal) animate(
	ctx context.Context,
	l log.Logger,
	text string,
	start time.Time,
	finished <-chan struct{},
) {
	op := &operation{start: start, text: text}

	term.add(op)
	defer term.remove(l, op)

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	term.redraw(l)

	for waitTick(ctx, ticker.C, finished) {
		term.redraw(l)
	}
}

// inputRead records that the terminal echoed the line break of an answer, which
// leaves the cursor at the start of a row.
func (term *terminal) inputRead() {
	if !term.echoesInput {
		return
	}

	term.mu.Lock()
	defer term.mu.Unlock()

	term.midLine = false
	term.draw(time.Now())
}

// add puts `op` on the progress line.
func (term *terminal) add(op *operation) {
	term.mu.Lock()
	defer term.mu.Unlock()

	term.ops = append(term.ops, op)
}

// remove takes `op` off the progress line and clears the line, so what is
// logged next starts on an empty row. Operations still running draw it again
// on their next frame.
func (term *terminal) remove(l log.Logger, op *operation) {
	term.mu.Lock()

	term.ops = slices.DeleteFunc(term.ops, func(other *operation) bool {
		return other == op
	})
	term.clear()

	failure := term.unloggedFailure()

	term.mu.Unlock()

	logFailure(l, failure)
}

// redraw draws the progress line for the current time.
func (term *terminal) redraw(l log.Logger) {
	term.mu.Lock()

	term.draw(time.Now())

	failure := term.unloggedFailure()

	term.mu.Unlock()

	logFailure(l, failure)
}

// draw puts the progress line for `now` on the screen. The caller holds the lock.
func (term *terminal) draw(now time.Time) {
	if term.midLine || len(term.ops) == 0 {
		return
	}

	width := term.width()

	line := term.line(now, width)
	if line == "" {
		term.clear()

		return
	}

	if line == term.last {
		return
	}

	columns := runewidth.StringWidth(line)
	leftover := max(min(term.drawn, width-1)-columns, 0)

	term.write("\r" + line + strings.Repeat(" ", leftover))

	term.drawn = columns
	term.last = line
}

// clear wipes the progress line and leaves the cursor at the start of its row.
// It never writes more than the terminal is wide now, so a terminal that got
// narrower since the line was drawn is not made to wrap. What that terminal
// re-wrapped from the old line stays on screen. The caller holds the lock.
func (term *terminal) clear() {
	if term.drawn == 0 {
		return
	}

	columns := term.drawn
	if width := term.width(); width > 0 {
		columns = min(columns, width-1)
	}

	term.write("\r" + strings.Repeat(" ", columns) + "\r")

	term.drawn = 0
	term.last = ""
}

// line returns the progress line for `now` on a terminal `width` columns wide,
// cut to one column less than that so it never wraps. It is empty when the
// terminal is too narrow to draw on.
func (term *terminal) line(now time.Time, width int) string {
	if width < minWidth {
		return ""
	}

	head := term.ops[0]
	frame := term.frames[int(now.Sub(term.epoch)/frameInterval)%len(term.frames)]
	suffix := fmt.Sprintf(" (%ds)", int(now.Sub(head.start).Seconds()))

	if more := len(term.ops) - 1; more > 0 {
		suffix += fmt.Sprintf(" +%d more", more)
	}

	room := width - 1 - runewidth.StringWidth(frame) - 1 - runewidth.StringWidth(suffix)
	if room < minMessageWidth {
		return runewidth.Truncate(frame+suffix, width-1, "")
	}

	return frame + " " + runewidth.Truncate(printable(head.text), room, ellipsis) + suffix
}

// write sends `s` to the terminal. A failed write ends all further drawing.
// The caller holds the lock.
func (term *terminal) write(s string) {
	if term.failure != nil {
		return
	}

	if _, err := io.WriteString(term.out, s); err != nil {
		term.failure = err
	}
}

// unloggedFailure returns the write failure that ended drawing, once. The
// caller holds the lock.
func (term *terminal) unloggedFailure() error {
	if term.failure == nil || term.failureLogged {
		return nil
	}

	term.failureLogged = true

	return term.failure
}

// printable returns `text` with every control character replaced by a space,
// so a tab or a line break in a message cannot move the cursor off the row.
func printable(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}

		return char
	}, text)
}

// logFailure logs why the progress line is no longer drawn.
func logFailure(l log.Logger, failure error) {
	if failure == nil {
		return
	}

	l.Debugf("Progress is no longer drawn on the terminal: %v", failure)
}
