package tui

import (
	"errors"
	"fmt"
)

// ErrNoTerminal reports that a terminal user interface cannot start because
// the process has no interactive terminal to attach to (for example, in a CI
// job or another environment where stdin is redirected).
var ErrNoTerminal = errors.New("an interactive terminal is required")

// EnsureTTY verifies that the process has an interactive terminal to draw on,
// returning an error wrapping [ErrNoTerminal] when it does not, so a caller
// can fail fast, or fall back to a non-interactive path, instead of surfacing
// the library's raw TTY error.
func EnsureTTY(isTerminal func() bool) error {
	if isTerminal() {
		return nil
	}

	return fmt.Errorf("%w: stdin is not a terminal", ErrNoTerminal)
}
