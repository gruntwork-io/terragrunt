package page

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const spacebar = " "

// KeyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type KeyMap struct {
	viewport.KeyMap
	help help.Model

	Navigation key.Binding

	Choose key.Binding

	// Help toggle keybindings.
	Help key.Binding

	// The quit keybinding. This won't be caught when filtering.
	Quit key.Binding

	// The quit-no-matter-what keybinding. This will be caught when filtering.
	ForceQuit key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (keys KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Navigation, keys.Choose, keys.Help, keys.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (keys KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.PageDown, keys.PageUp}, // first column
		{keys.Navigation, keys.Choose},                   // first column
		{keys.Help, keys.Quit, keys.ForceQuit},           // second column
	}
}

func (keys *KeyMap) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Help):
			keys.help.ShowAll = !keys.help.ShowAll
		}
	}

	return tea.Batch(cmds...)
}

func (keys *KeyMap) View() string {
	return lipgloss.NewStyle().Padding(2, 0, 0, 2).Render(keys.help.View(keys))
}

func newKeyMap() KeyMap {
	return KeyMap{
		help: help.New(),
		KeyMap: viewport.KeyMap{
			HalfPageUp: key.NewBinding(
				key.WithDisabled(),
			),
			HalfPageDown: key.NewBinding(
				key.WithDisabled(),
			),
			Up: key.NewBinding(
				key.WithKeys("up", "ctrl+p"),
				key.WithHelp("↑/ctrl+p", "move up"),
			),
			Down: key.NewBinding(
				key.WithKeys("down", "ctrl+n"),
				key.WithHelp("↓/ctrl+n", "move down"),
			),
			PageDown: key.NewBinding(
				key.WithKeys("right", "pgdown", "ctrl+v"),
				key.WithHelp("→/pgdn/ctrl+v", "page down"),
			),
			PageUp: key.NewBinding(
				key.WithKeys("left", "pgup", "alt+v"),
				key.WithHelp("←/pgup/alt+v", "page up"),
			),
		},
		Navigation: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "navigation"),
		),
		Choose: key.NewBinding(
			key.WithKeys("enter", "ctrl-j"),
			key.WithHelp("enter/ctrl-j", "choose"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q", "back to list"),
		),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
	}
}
