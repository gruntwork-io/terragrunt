package tui

import (
	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/list"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/lipgloss/v2/compat"
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
		Foreground(compat.AdaptiveColor{Dark: lipgloss.Color(selectedTitleForegroundColorDark)}).
		BorderForeground(compat.AdaptiveColor{Dark: lipgloss.Color(selectedTitleBorderForegroundColorDark)})

	d.Styles.SelectedDesc = d.Styles.SelectedTitle.
		Foreground(compat.AdaptiveColor{Dark: lipgloss.Color(selectedDescForegroundColorDark)}).
		BorderForeground(compat.AdaptiveColor{Dark: lipgloss.Color(selectedDescBorderForegroundColorDark)})

	help := []key.Binding{keys.choose, keys.scaffold}

	d.ShortHelpFunc = func() []key.Binding {
		return help
	}

	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}

	return d
}
