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
	TabTemplates
	TabStacks
	TabUnits
	TabModules
	numTabs
)

// String returns the user-visible tab label.
func (t tabKind) String() string {
	switch t {
	case TabModules:
		return "Modules"
	case TabTemplates:
		return "Templates"
	case TabUnits:
		return "Units"
	case TabStacks:
		return "Stacks"
	case TabAll, numTabs:
		return "All"
	default:
		return "All"
	}
}

// matches reports whether the given entry belongs in this tab. A component
// is in a kind-specific tab when its Kind matches OR when one of its
// front-matter tags case-insensitively names the tab's kind (so a
// `template` tagged `module` shows up in both Templates and Modules).
// TabAll matches everything.
func (t tabKind) matches(entry *ComponentEntry) bool {
	if entry == nil {
		return t == TabAll
	}

	switch t {
	case TabModules:
		return entry.Kind() == ComponentKindModule || entry.HasTagForKind(ComponentKindModule)
	case TabTemplates:
		return entry.Kind() == ComponentKindTemplate || entry.HasTagForKind(ComponentKindTemplate)
	case TabUnits:
		return entry.Kind() == ComponentKindUnit || entry.HasTagForKind(ComponentKindUnit)
	case TabStacks:
		return entry.Kind() == ComponentKindStack || entry.HasTagForKind(ComponentKindStack)
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

// componentKind returns the ComponentKind this tab filters to. TabAll and
// the numTabs sentinel return false.
func (t tabKind) componentKind() (ComponentKind, bool) {
	switch t {
	case TabModules:
		return ComponentKindModule, true
	case TabTemplates:
		return ComponentKindTemplate, true
	case TabUnits:
		return ComponentKindUnit, true
	case TabStacks:
		return ComponentKindStack, true
	case TabAll, numTabs:
		return 0, false
	}

	return 0, false
}

// tabBarInactiveStyle is the style used for non-active tabs in the tab strip.
var tabBarInactiveStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6C7086")).
	Padding(0, 1)

// tabActiveStyle returns the active-tab style: kind tabs use their pill
// colors, TabAll uses the neutral title style.
func tabActiveStyle(t tabKind) lipgloss.Style {
	kind, ok := t.componentKind()
	if !ok {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(titleForegroundColor)).
			Background(lipgloss.Color(titleBackgroundColor)).
			Bold(true).
			Padding(0, 1)
	}

	bg, fg := pillColorsForKind(kind, false)

	return lipgloss.NewStyle().
		Background(lipgloss.Color(bg)).
		Foreground(lipgloss.Color(fg)).
		Bold(true).
		Padding(0, 1)
}

// RenderTabBar returns the tab strip with the active tab highlighted. When
// loading is true, a "(loading...)" suffix trails the strip.
func RenderTabBar(active tabKind, loading bool) string {
	var parts []string

	for i := range int(numTabs) {
		t := tabKind(i)
		label := t.String()

		if t == active {
			parts = append(parts, tabActiveStyle(t).Render(label))

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
