package md

import (
	"charm.land/glamour/v2"
)

// DocumentMargin is the left margin a rendered document is indented by.
// Content prepended to rendered output carries the same indent, so that it
// lines up with the body it opens.
const DocumentMargin = 2

// NoWrapWidth is the wrap column that stands in for wrapping turned off.
// Word-wrap cannot be switched off outright (a width of `0` collapses the
// document), so a column nothing reaches preserves the author's line breaks
// and leaves anything that overruns the terminal to the caller.
const NoWrapWidth = 1 << 14

// Background is the brightness of the terminal that rendered output is drawn
// on, which decides the style it is drawn in.
type Background int

const (
	// LightBackground styles output for a light terminal.
	LightBackground Background = iota
	// DarkBackground styles output for a dark terminal.
	DarkBackground
)

// style names the standard style b is rendered with.
func (b Background) style() string {
	if b == DarkBackground {
		return "dark"
	}

	return "light"
}

// TerminalRenderer renders Markdown as styled terminal output.
type TerminalRenderer struct {
	renderer *glamour.TermRenderer
}

// NewTerminalRenderer returns a renderer that word-wraps at width and styles
// its output for bg.
func NewTerminalRenderer(width int, bg Background) (*TerminalRenderer, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(bg.style()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	return &TerminalRenderer{renderer: renderer}, nil
}

// Render renders source as styled terminal output.
func (r *TerminalRenderer) Render(source string) (string, error) {
	return r.renderer.Render(source)
}
