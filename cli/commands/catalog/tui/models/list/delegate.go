package list

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

const (
	selectedTitleForegroundColorDark       = "#63C5DA"
	selectedTitleBorderForegroundColorDark = "#63C5DA"

	selectedDescForegroundColorDark       = "#59788E"
	selectedDescBorderForegroundColorDark = "#63C5DA"

	statusMessageForegroundColorDark = "#04B575"
)

type DefaultDelegate struct {
	list.DefaultDelegate
}

type Delegate struct {
	DefaultDelegate
	*DelegateKeyMap
}

func NewDelegate() *Delegate {
	defaultDelegate := list.NewDefaultDelegate()
	defaultDelegate.Styles.SelectedTitle.
		Foreground(lipgloss.AdaptiveColor{Dark: selectedTitleForegroundColorDark}).
		BorderForeground(lipgloss.AdaptiveColor{Dark: selectedTitleBorderForegroundColorDark})

	defaultDelegate.Styles.SelectedDesc = defaultDelegate.Styles.SelectedTitle.Copy().
		Foreground(lipgloss.AdaptiveColor{Dark: selectedDescForegroundColorDark}).
		BorderForeground(lipgloss.AdaptiveColor{Dark: selectedDescBorderForegroundColorDark})

	return &Delegate{
		DefaultDelegate: DefaultDelegate{defaultDelegate},
		DelegateKeyMap:  NewDelegateKeyMap(),
	}
}
