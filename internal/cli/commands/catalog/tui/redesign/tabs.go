package redesign

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// tabKind identifies which catalog component type the active tab is
// filtering by. TabAll shows every component.
type tabKind int

const (
	TabAll tabKind = iota
	TabModules
	TabTemplates
	numTabs
)

// String returns the user-visible tab label.
func (t tabKind) String() string {
	switch t {
	case TabModules:
		return "Modules"
	case TabTemplates:
		return "Templates"
	case TabAll, numTabs:
		return "All"
	default:
		return "All"
	}
}

// matches reports whether a component of the given kind belongs in this tab.
// TabAll matches everything.
func (t tabKind) matches(kind ComponentKind) bool {
	switch t {
	case TabModules:
		return kind == ComponentKindModule
	case TabTemplates:
		return kind == ComponentKindTemplate
	case TabAll, numTabs:
		return true
	default:
		return true
	}
}

// next cycles to the following tab, wrapping around past the last one.
func (t tabKind) next() tabKind {
	return (t + 1) % numTabs
}

// prev cycles to the previous tab, wrapping around past the first one.
func (t tabKind) prev() tabKind {
	return (t + numTabs - 1) % numTabs
}

// Tab bar styling.
var (
	tabBarActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(titleForegroundColor)).
				Background(lipgloss.Color(titleBackgroundColor)).
				Bold(true).
				Padding(0, 1)

	tabBarInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6C7086")).
				Padding(0, 1)
)

// renderTabBar returns the tab strip with the active tab highlighted. When
// loading is true, a "(loading...)" suffix trails the strip.
func renderTabBar(active tabKind, loading bool) string {
	var parts []string

	for i := range int(numTabs) {
		t := tabKind(i)
		label := t.String()

		if t == active {
			parts = append(parts, tabBarActiveStyle.Render(label))

			continue
		}

		parts = append(parts, tabBarInactiveStyle.Render(label))
	}

	bar := strings.Join(parts, " ")

	if loading {
		bar += tabBarInactiveStyle.Render("(loading...)")
	}

	return bar
}
