package venv

import (
	"os"

	"golang.org/x/term"
)

// Terminal reports the console a run's output is adapting to: whether each
// standard stream is attached to a terminal, and how wide that terminal is.
// Width reports 0 when no width is available, which callers read as "do not
// wrap" or replace with a default of their own.
type Terminal struct {
	StdinIsTTY  func() bool
	StdoutIsTTY func() bool
	StderrIsTTY func() bool
	Width       func() int
}

// RequireTerminal panics with [ErrVenvTerminalUnset] when Terminal is nil.
// Functions that size or color their output call this as their first
// statement so a missing handle panics at the offending call site instead of
// inside an unrelated stack frame.
func (v *Venv) RequireTerminal() {
	if v.Terminal == nil {
		panic(ErrVenvTerminalUnset)
	}
}

// osTerminalWidth reports the real terminal's width, and 0 when stdout is not
// a terminal (a pipe, a file, a CI log). Callers that need a number either way
// substitute their own default for the 0.
func osTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}

	return width
}
