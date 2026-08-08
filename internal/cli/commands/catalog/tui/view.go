package tui

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
	loadNoticeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(valuesBoxAccentYellow))
)

// View implements bubbletea.Model.View, dispatching to a sub-view based on
// the current session state.
func (m Model) View() tea.View {
	var s string

	switch m.State {
	case ListState:
		s = m.listView()
	case PagerState:
		s = m.pagerView()
	case FormState:
		s = m.formView()
	case ScaffoldState:
	default:
		s = ""
	}

	v := tea.NewView(m.toasts.Overlay(s, m.width, m.height))
	v.AltScreen = true

	return v
}

func (m Model) listView() string {
	bar := RenderTabBar(m.activeTab, m.loading)
	active := m.lists[m.activeTab]

	// The blank spacer between the tab strip and the list doubles as a
	// status line: partial source-load failures render there, so the
	// height math in the WindowSizeMsg handler stays unchanged.
	notice := ""
	if m.loadErr != nil {
		notice = loadNoticeStyle.Render("⚠ " + m.loadErr.Error())
	}

	return lipgloss.JoinVertical(lipgloss.Left, bar, notice, active.View())
}

func (m Model) pagerView() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), m.footerView())
}

// formView renders the interactive value-collection form. Until the
// discovery goroutine produces a formReadyMsg the form pointer is nil and
// we render a loading hint so the user knows the TUI is working.
func (m Model) formView() string {
	if m.form == nil {
		return formMetaStyle.Render("Discovering variables…")
	}

	return m.form.View()
}

func (m Model) footerView() string {
	var percent float64 = 100

	info := infoPositionStyle.Render(fmt.Sprintf("%2.f%%", m.viewport.ScrollPercent()*percent))

	line := strings.Repeat("─", max(0, m.viewport.Width()-lipgloss.Width(info)))
	line = infoLineStyle.Render(line)

	info = lipgloss.JoinHorizontal(lipgloss.Center, line, info)

	pagerKeys := infoHelp.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.buttonBar.View().Content,
			"\n",
			m.pagerKeys.HelpModel.View(m.pagerKeys),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left, info, pagerKeys)
}
