package list

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// NewKeyMap returns a set of keybindings.
func NewKeyMap() list.KeyMap {
	return list.KeyMap{
		// Browsing.
		CursorUp: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "move up"),
		),
		CursorDown: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "move down"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("left", "pgup", "alt+v"),
			key.WithHelp("←/pgup/alt+v", "prev page"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("right", "pgdown", "ctrl+v"),
			key.WithHelp("→/pgdn/ctrl+v", "next page"),
		),
		GoToStart: key.NewBinding(
			key.WithKeys("home", "ctrl+a"),
			key.WithHelp("home/ctrl+a", "go to start"),
		),
		GoToEnd: key.NewBinding(
			key.WithKeys("end", "ctrl+e"),
			key.WithHelp("end/ctrl+e", "go to end"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear filter"),
		),

		// Filtering.
		CancelWhileFiltering: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		AcceptWhileFiltering: key.NewBinding(
			key.WithKeys("enter", "tab", "shift+tab", "ctrl+k", "up", "ctrl+j", "down"),
			key.WithHelp("enter", "apply filter"),
		),

		// Toggle help.
		ShowFullHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "more"),
		),
		CloseFullHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "close help"),
		),

		// Quitting.
		Quit: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q", "quit"),
		),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
	}
}
