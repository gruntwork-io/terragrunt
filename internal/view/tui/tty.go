package tui

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// ErrNoTerminal reports that a terminal user interface cannot start because
// the process has no interactive terminal to attach to (for example, in a CI
// job or another environment where stdin is redirected).
var ErrNoTerminal = errors.New("an interactive terminal is required")

// EnsureTTY verifies that the process has an interactive terminal to draw on,
// returning an error wrapping [ErrNoTerminal] when it does not, so a caller
// can fail fast, or fall back to a non-interactive path, instead of surfacing
// the library's raw TTY error.
//
// A terminal stdin is the whole test. Reaching for a terminal that stdin does
// not point at, the way bubbletea does when it opens /dev/tty or CONIN$, finds
// one for processes nobody is sitting at: every Windows process holding a
// console can open CONIN$, and a command a tool or a script runs inherits the
// terminal of the session that launched it. Both would draw a form and wait on
// input that never comes.
func EnsureTTY(isTerminal func() bool) error {
	if isTerminal() {
		return nil
	}

	return fmt.Errorf("%w: stdin is not a terminal", ErrNoTerminal)
}

// EnsureOSTTY runs [EnsureTTY] against the real process environment.
func EnsureOSTTY() error {
	return EnsureTTY(stdinIsTerminal)
}

// stdinIsTerminal reports whether the process's stdin is a terminal.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
