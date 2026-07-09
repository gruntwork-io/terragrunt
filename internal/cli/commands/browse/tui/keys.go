package tui

import "charm.land/bubbles/v2/key"

// keyMap holds the Miller-columns navigation bindings: vim keys and arrows,
// plus the incremental search bindings.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Top       key.Binding
	Home      key.Binding
	Bottom    key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Ascend    key.Binding
	Descend   key.Binding
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Quit      key.Binding
}

// newKeyMap returns the default navigation bindings.
//
// Some of these don't have help because they aren't given hints in the TUI.
// This is to avoid cluttering the UI. They're documented in the docs instead.
func newKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
		),
		Ascend: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/←", "back"),
		),
		Descend: key.NewBinding(
			key.WithKeys("l", "right", "enter"),
			key.WithHelp("l/→", "select"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "previous match"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns the bindings shown in the footer's help line.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Ascend, k.Descend, k.Search, k.Quit}
}
