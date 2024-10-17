package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

const (
	selectedTitleForegroundColorDark       = "#63C5DA"
	selectedTitleBorderForegroundColorDark = "#63C5DA"

	selectedDescForegroundColorDark       = "#59788E"
	selectedDescBorderForegroundColorDark = "#63C5DA"
)

func newItemDelegate(keys *delegateKeyMap) list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.SelectedTitle.
		Foreground(lipgloss.AdaptiveColor{Dark: selectedTitleForegroundColorDark}).
		BorderForeground(lipgloss.AdaptiveColor{Dark: selectedTitleBorderForegroundColorDark})

	d.Styles.SelectedDesc = d.Styles.SelectedTitle.
		Foreground(lipgloss.AdaptiveColor{Dark: selectedDescForegroundColorDark}).
		BorderForeground(lipgloss.AdaptiveColor{Dark: selectedDescBorderForegroundColorDark})

	help := []key.Binding{keys.choose, keys.scaffold}

	d.ShortHelpFunc = func() []key.Binding {
		return help
	}

	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}

	return d
}
