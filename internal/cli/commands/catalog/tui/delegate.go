package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

const (
	selectedTitleForegroundColorDark       = "#63C5DA"
	selectedTitleBorderForegroundColorDark = "#63C5DA"

	selectedDescForegroundColorDark       = "#59788E"
	selectedDescBorderForegroundColorDark = "#63C5DA"
)

func newItemDelegate(keys *delegateKeyMap) list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(selectedTitleForegroundColorDark)).
		BorderForeground(lipgloss.Color(selectedTitleBorderForegroundColorDark))

	d.Styles.SelectedDesc = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(selectedDescForegroundColorDark)).
		BorderForeground(lipgloss.Color(selectedDescBorderForegroundColorDark))

	help := []key.Binding{keys.choose, keys.scaffold}

	d.ShortHelpFunc = func() []key.Binding {
		return help
	}

	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}

	return d
}
