package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
)

// newListKeyMap returns a set of keybindings for the list view.
func newListKeyMap() list.KeyMap {
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

type delegateKeyMap struct {
	choose   key.Binding
	scaffold key.Binding
}

// Additional short help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d delegateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		d.choose,
		d.scaffold,
	}
}

// Additional full help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d delegateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			d.choose,
			d.scaffold,
		},
	}
}

// newDelegateKeyMap returns a set of keybindings.
func newDelegateKeyMap() *delegateKeyMap {
	return &delegateKeyMap{
		choose: key.NewBinding(
			key.WithKeys("enter", "ctrl-j"),
			key.WithHelp("enter/ctrl-j", "choose"),
		),
		scaffold: key.NewBinding(
			key.WithKeys("S", "s"),
			key.WithHelp("S", "Scaffold"),
		),
	}
}

// pagerKeyMap returns a set of keybindings for the pager. It satisfies to the
// help.KeyMap interface, which is used to render the menu.
type pagerKeyMap struct {
	viewport.KeyMap

	help help.Model

	// Button navigation
	Navigation key.Binding

	// Button navigation
	NavigationBack key.Binding

	// Select button
	Choose key.Binding

	// Run Scaffold command
	Scaffold key.Binding

	// Help toggle keybindings.
	Help key.Binding

	// The quit keybinding. This won't be caught when filtering.
	Quit key.Binding

	// The quit-no-matter-what keybinding. This will be caught when filtering.
	ForceQuit key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (keys pagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		keys.Up,
		keys.Down,
		keys.Navigation,
		keys.NavigationBack,
		keys.Choose,
		keys.Scaffold,
		keys.Help,
		keys.Quit,
	}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (keys pagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.PageDown, keys.PageUp},                   // first column
		{keys.Navigation, keys.NavigationBack, keys.Choose, keys.Scaffold}, // second column
		{keys.Help, keys.Quit, keys.ForceQuit},                             // third column
	}
}

// newPagerKeyMap returns a set of keybindings for the pager view.
func newPagerKeyMap() pagerKeyMap {
	return pagerKeyMap{
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
		help: help.New(),
		Navigation: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "navigation"),
		),
		NavigationBack: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "navigation"),
		),
		Choose: key.NewBinding(
			key.WithKeys("enter", "ctrl-j"),
			key.WithHelp("enter/ctrl-j", "choose"),
		),
		Scaffold: key.NewBinding(
			key.WithKeys("S", "s"),
			key.WithHelp("S", "Scaffold"),
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
