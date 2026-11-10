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

const (
	frameInterval   = 100 * time.Millisecond
	minWidth        = 20
	ellipsis        = "..."
	gutter          = " "
	eraseDown       = "\x1b[J"
	minMessageWidth = len(ellipsis) + 1
)

var (
	unicodeFrames = []string{
		"\u2801", "\u2802", "\u2804", "\u280d", "\u2816", "\u281e", "\u281e",
		"\u281e", "\u281e", "\u281e", "\u281e", "\u2834", "\u2820", "\u2800",
		"\u2809", "\u2812", "\u281b", "\u281b", "\u281b", "\u281b",
		"\u281b", "\u281b", "\u2836", "\u2824", "\u2800",
	}
	asciiFrames = []string{"|", "/", "-", "\\"}
)

type terminal struct {
	epoch         time.Time
	out           io.Writer
	failure       error
	width         func() int
	last          string
	frames        []string
	ops           []*operation
	mu            sync.Mutex
	shown         bool
	midLine       bool
	failureLogged bool
	echoesInput   bool
}

type operation struct {
	start time.Time
	text  string
}

type guard struct {
	term *terminal
	w    io.Writer
}

// Write clears the progress line, writes p, and draws the line again if p ends with a line break.
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

func newTerminal(out io.Writer, width func() int, frames []string) *terminal {
	return &terminal{
		epoch:  time.Now(),
		out:    out,
		width:  width,
		frames: frames,
	}
}

// animate adds the operation to the progress line and redraws it until the operation finishes or ctx is cancelled.
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

// inputRead draws the progress line again once a prompt was answered on a terminal that echoes input.
func (term *terminal) inputRead() {
	if !term.echoesInput {
		return
	}

	term.mu.Lock()
	defer term.mu.Unlock()

	term.midLine = false
	term.draw(time.Now())
}

func (term *terminal) add(op *operation) {
	term.mu.Lock()
	defer term.mu.Unlock()

	term.ops = append(term.ops, op)
}

// remove drops op and clears the progress line.
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

func (term *terminal) redraw(l log.Logger) {
	term.mu.Lock()

	term.draw(time.Now())

	failure := term.unloggedFailure()

	term.mu.Unlock()

	logFailure(l, failure)
}

// draw writes the progress line and erases the rows below it, with term.mu held.
func (term *terminal) draw(now time.Time) {
	if term.midLine || len(term.ops) == 0 {
		return
	}

	line := term.line(now, term.width())
	if line == "" {
		term.clear()

		return
	}

	if line == term.last {
		return
	}

	term.write("\r" + line + eraseDown + "\r")

	term.shown = true
	term.last = line
}

// clear erases the progress line and the rows below it, with term.mu held.
func (term *terminal) clear() {
	if !term.shown {
		return
	}

	term.write("\r" + eraseDown)

	term.shown = false
	term.last = ""
}

// line returns the progress line cut to width-1 columns, or "" if the terminal is too narrow.
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

	room := width - 1 - len(gutter) - runewidth.StringWidth(frame) - 1 - runewidth.StringWidth(suffix)
	if room < minMessageWidth {
		return runewidth.Truncate(gutter+frame+suffix, width-1, "")
	}

	return gutter + frame + " " + runewidth.Truncate(printable(head.text), room, ellipsis) + suffix
}

// write writes s to the terminal until a write fails, with term.mu held.
func (term *terminal) write(s string) {
	if term.failure != nil {
		return
	}

	if _, err := io.WriteString(term.out, s); err != nil {
		term.failure = err
	}
}

// unloggedFailure returns the write error the first time only, with term.mu held.
func (term *terminal) unloggedFailure() error {
	if term.failure == nil || term.failureLogged {
		return nil
	}

	term.failureLogged = true

	return term.failure
}

// printable replaces control characters with spaces.
func printable(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}

		return char
	}, text)
}

// logFailure logs a failed terminal write at debug level, and nothing for a nil failure.
func logFailure(l log.Logger, failure error) {
	if failure == nil {
		return
	}

	l.Debugf("Progress is no longer drawn on the terminal: %v", failure)
}
