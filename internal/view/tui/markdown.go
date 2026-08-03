// Package tui holds styling and rendering helpers shared by Terragrunt's
// interactive TUIs, such as catalog and browse.
package tui

import (
	"charm.land/glamour/v2"
)

// NewMarkdownRenderer returns a glamour renderer that word-wraps at width and
// uses the standard dark or light style to match the terminal background.
func NewMarkdownRenderer(width int, dark bool) (*glamour.TermRenderer, error) {
	style := "dark"
	if !dark {
		style = "light"
	}

	return glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
}
