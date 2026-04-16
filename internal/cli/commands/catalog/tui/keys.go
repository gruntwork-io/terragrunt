package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
)

// NewListKeyMap returns a set of keybindings for the list view.
func NewListKeyMap() list.KeyMap {
	return list.KeyMap{
		// Browsing.
		CursorUp: key.NewBinding(
			key.WithKeys("k", "up", "ctrl+p"),
			key.WithHelp("k/↑/ctrl+p", "move up"),
		),
		CursorDown: key.NewBinding(
			key.WithKeys("j", "down", "ctrl+n"),
			key.WithHelp("j/↓/ctrl+n", "move down"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("h", "left", "pgup", "alt+v"),
			key.WithHelp("h/←/pgup/alt+v", "prev page"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("l", "right", "pgdown", "ctrl+v"),
			key.WithHelp("l/→/pgdn/ctrl+v", "next page"),
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

type DelegateKeyMap struct {
	Choose   key.Binding
	Scaffold key.Binding
}

// Additional short help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d DelegateKeyMap) ShortHelp() []key.Binding { //nolint:gocritic
	return []key.Binding{
		d.Choose,
		d.Scaffold,
	}
}

// Additional full help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d DelegateKeyMap) FullHelp() [][]key.Binding { //nolint:gocritic
	return [][]key.Binding{
		{
			d.Choose,
			d.Scaffold,
		},
	}
}

// NewDelegateKeyMap returns a set of keybindings.
func NewDelegateKeyMap() *DelegateKeyMap {
	return &DelegateKeyMap{
		Choose: key.NewBinding(
			key.WithKeys("enter", "ctrl-j"),
			key.WithHelp("enter/ctrl-j", "choose"),
		),
		Scaffold: key.NewBinding(
			key.WithKeys("S", "s"),
			key.WithHelp("S", "Scaffold"),
		),
	}
}

// PagerKeyMap returns a set of keybindings for the pager. It satisfies to the
// help.KeyMap interface, which is used to render the menu.
type PagerKeyMap struct {
	viewport.KeyMap

	HelpModel help.Model

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
func (keys PagerKeyMap) ShortHelp() []key.Binding { //nolint:gocritic
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
func (keys PagerKeyMap) FullHelp() [][]key.Binding { //nolint:gocritic
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.PageDown, keys.PageUp},                   // first column
		{keys.Navigation, keys.NavigationBack, keys.Choose, keys.Scaffold}, // second column
		{keys.Help, keys.Quit, keys.ForceQuit},                             // third column
	}
}

// NewPagerKeyMap returns a set of keybindings for the pager view.
func NewPagerKeyMap() PagerKeyMap {
	return PagerKeyMap{
		KeyMap: viewport.KeyMap{
			HalfPageUp: key.NewBinding(
				key.WithDisabled(),
			),
			HalfPageDown: key.NewBinding(
				key.WithDisabled(),
			),
			Up: key.NewBinding(
				key.WithKeys("k", "up", "ctrl+p"),
				key.WithHelp("k/↑/ctrl+p", "move up"),
			),
			Down: key.NewBinding(
				key.WithKeys("j", "down", "ctrl+n"),
				key.WithHelp("j/↓/ctrl+n", "move down"),
			),
			PageDown: key.NewBinding(
				key.WithKeys("l", "right", "pgdown", "ctrl+v"),
				key.WithHelp("l/→/pgdn/ctrl+v", "page down"),
			),
			PageUp: key.NewBinding(
				key.WithKeys("h", "left", "pgup", "alt+v"),
				key.WithHelp("h/←/pgup/alt+v", "page up"),
			),
		},
		HelpModel: help.New(),
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
