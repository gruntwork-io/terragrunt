package list

import "github.com/charmbracelet/bubbles/key"

// DelegateKeyMap defines keybindings. It satisfies to the help.DelegateKeyMap interface, which
// is used to render the menu.
type DelegateKeyMap struct {
	Choose key.Binding
}

// NewDelegateKeyMap returns a set of keybindings.
func NewDelegateKeyMap() *DelegateKeyMap {
	return &DelegateKeyMap{
		Choose: key.NewBinding(
			key.WithKeys("enter", "ctrl-j"),
			key.WithHelp("enter/ctrl-j", "choose"),
		),
	}
}

// Additional short help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d DelegateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		d.Choose,
	}
}

// Additional full help entries. This satisfies the help.KeyMap interface and
// is entirely optional.
func (d DelegateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			d.Choose,
		},
	}
}
