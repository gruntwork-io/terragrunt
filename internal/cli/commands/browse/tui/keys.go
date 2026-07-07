package tui

import "charm.land/bubbles/v2/key"

// keyMap holds the Miller-columns navigation bindings: vim keys and arrows,
// plus the incremental search bindings.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Top       key.Binding
	Bottom    key.Binding
	Ascend    key.Binding
	Descend   key.Binding
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Quit      key.Binding
}

// newKeyMap returns the default navigation bindings.
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
		// Top and Bottom are the vim gg/G jumps. They carry no help text on
		// purpose: they're documented in the command docs but kept out of the
		// footer hints.
		Top: key.NewBinding(
			key.WithKeys("g"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
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
