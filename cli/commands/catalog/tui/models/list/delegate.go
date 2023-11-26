package list

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// Update checks whether the delegate's UpdateFunc is set and calls it.
func (delegate Delegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	var title string

	if i, ok := m.SelectedItem().(list.DefaultItem); ok {
		title = i.Title()
	} else {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, delegate.Choose):
			statusMessage := lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Dark: statusMessageForegroundColorDark}).
				Render("You chose " + title)

			return m.NewStatusMessage(statusMessage)
		}
	}

	return nil
}
