package redesign

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	// bodyPaddingVertical and bodyPaddingHorizontal define the padding applied
	// to the main content body across all redesign views so it breathes from
	// the terminal edge without burying the view logic in literal numbers.
	bodyPaddingVertical   = 1
	bodyPaddingHorizontal = 2

	// infoHelpPaddingTop is the extra vertical padding above the help line so
	// it sits apart from the surrounding content.
	infoHelpPaddingTop = 2
)

var (
	AppStyle          = lipgloss.NewStyle().Padding(bodyPaddingVertical, bodyPaddingHorizontal)
	infoPositionStyle = lipgloss.NewStyle().Padding(0, 1).BorderStyle(lipgloss.HiddenBorder())
	infoLineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#1D252F"))
	infoHelp          = lipgloss.NewStyle().Padding(infoHelpPaddingTop, 0, 0, bodyPaddingHorizontal)
)

// View is the main view, which just calls the appropriate sub-view and returns a View representation of the TUI
// based on the application's state.
func (m Model) View() tea.View {
	var s string

	switch m.State {
	case ListState:
		s = m.listView()
	case PagerState:
		s = m.pagerView()
	case ScaffoldState:
	default:
		s = ""
	}

	v := tea.NewView(s)
	v.AltScreen = true

	return v
}

func (m Model) listView() string {
	bar := RenderTabBar(m.activeTab, m.loading)
	active := m.lists[m.activeTab]

	return lipgloss.JoinVertical(lipgloss.Left, bar, "", active.View())
}

func (m Model) pagerView() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), m.footerView())
}

func (m Model) footerView() string {
	var percent float64 = 100

	info := infoPositionStyle.Render(fmt.Sprintf("%2.f%%", m.viewport.ScrollPercent()*percent))

	line := strings.Repeat("─", max(0, m.viewport.Width()-lipgloss.Width(info)))
	line = infoLineStyle.Render(line)

	info = lipgloss.JoinHorizontal(lipgloss.Center, line, info)

	// button bar and key help
	pagerKeys := infoHelp.Render(lipgloss.JoinVertical(lipgloss.Left, m.buttonBar.View().Content, "\n", m.pagerKeys.HelpModel.View(m.pagerKeys)))

	return lipgloss.JoinVertical(lipgloss.Left, info, pagerKeys)
}
