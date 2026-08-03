package tui

import (
	"errors"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ErrNoTerminal reports that the catalog TUI cannot start because the
// process has no interactive terminal to attach to (for example, in a CI
// job or another environment without a controlling terminal).
var ErrNoTerminal = errors.New("the catalog command requires an interactive terminal")

// EnsureTTY verifies that the process can attach an interactive terminal,
// mirroring bubbletea's input setup: a terminal stdin is used directly;
// otherwise the controlling terminal is opened. It returns an error
// wrapping [ErrNoTerminal] when neither is available, so the catalog
// command can fail fast with a clear message instead of surfacing the
// library's raw TTY error.
//
// isTerminal and openTTY are injected so tests can simulate environments
// without a controlling terminal; production callers use [EnsureOSTTY].
func EnsureTTY(
	l log.Logger,
	isTerminal func() bool,
	openTTY func() (io.Closer, io.Closer, error),
) error {
	if isTerminal() {
		return nil
	}

	in, out, err := openTTY()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}

	// The probe only checks availability; bubbletea opens its own handles
	// later, so release these right away. On POSIX systems both handles
	// are the same /dev/tty file, so close it once.
	if cerr := in.Close(); cerr != nil {
		l.Debugf("Failed to close TTY probe input: %v", cerr)
	}

	if out != in {
		if cerr := out.Close(); cerr != nil {
			l.Debugf("Failed to close TTY probe output: %v", cerr)
		}
	}

	return nil
}

// EnsureOSTTY runs [EnsureTTY] against the real process environment.
func EnsureOSTTY(l log.Logger) error {
	return EnsureTTY(l, stdinIsTerminal, openOSTTY)
}

// openOSTTY adapts [tea.OpenTTY] to EnsureTTY's probe signature.
func openOSTTY() (io.Closer, io.Closer, error) {
	return tea.OpenTTY()
}

// stdinIsTerminal reports whether the process's stdin is a terminal.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
