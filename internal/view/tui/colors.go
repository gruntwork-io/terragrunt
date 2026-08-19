// Package tui holds styling and rendering helpers shared by Terragrunt's
// interactive TUIs, such as catalog and browse.
package tui

const (
	// SelectionBlue is Terragrunt's interactive-selection blue: the accent
	// shared across TUIs for the selected list item, focused cursor, and
	// related highlights, so every interactive screen reads as one UI.
	SelectionBlue = "#63C5DA"

	// SelectionText is the dark slate drawn on top of SelectionBlue, e.g.
	// the label text on a full-width selection bar.
	SelectionText = "#1D252F"

	// TitleForeground and TitleBackground are the muted grey on slate of a
	// title strip, shared by every screen that draws one.
	TitleForeground = "#A8ACB1"
	TitleBackground = "#1D252F"
)
