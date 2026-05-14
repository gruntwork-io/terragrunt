package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	AppStyle          = lipgloss.NewStyle().Padding(1, 2) //nolint:mnd
	infoPositionStyle = lipgloss.NewStyle().Padding(0, 1).BorderStyle(lipgloss.HiddenBorder())
	infoLineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#1D252"))
	infoHelp          = lipgloss.NewStyle().Padding(2, 0, 0, 2) //nolint:mnd
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
	return m.List.View()
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
