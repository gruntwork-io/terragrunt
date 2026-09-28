package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
)

// TabKind identifies which catalog component type the active tab is
// filtering by. TabAll shows every component.
type TabKind int

const (
	TabAll TabKind = iota
	TabTemplates
	TabStacks
	TabUnits
	TabModules
	numTabs
)

// String returns the user-visible tab label.
func (t TabKind) String() string {
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
	}

	return "All"
}

// Matches reports whether the given non-nil entry belongs in this tab.
// A component is in a kind-specific tab when its Kind matches OR when
// one of its front-matter tags case-insensitively names the tab's kind
// (so a `template` tagged `module` shows up in both Templates and
// Modules). TabAll matches everything.
func (t TabKind) Matches(entry *ComponentEntry) bool {
	switch t {
	case TabModules:
		return entry.Kind() == component.KindModule || entry.HasTagForKind(component.KindModule)
	case TabTemplates:
		return entry.Kind() == component.KindTemplate || entry.HasTagForKind(component.KindTemplate)
	case TabUnits:
		return entry.Kind() == component.KindUnit || entry.HasTagForKind(component.KindUnit)
	case TabStacks:
		return entry.Kind() == component.KindStack || entry.HasTagForKind(component.KindStack)
	case TabAll, numTabs:
	}

	return true
}

// next cycles to the following tab, wrapping around past the last one.
func (t TabKind) next() TabKind {
	return (t + 1) % numTabs
}

// prev cycles to the previous tab, wrapping around past the first one.
func (t TabKind) prev() TabKind {
	return (t + numTabs - 1) % numTabs
}

// componentKind returns the component.Kind this tab filters to. TabAll returns
// false to signal "no kind filter".
func (t TabKind) componentKind() (component.Kind, bool) {
	switch t {
	case TabModules:
		return component.KindModule, true
	case TabTemplates:
		return component.KindTemplate, true
	case TabUnits:
		return component.KindUnit, true
	case TabStacks:
		return component.KindStack, true
	case TabAll, numTabs:
	}

	return 0, false
}

// tabBarInactiveStyle is the style used for non-active tabs in the tab strip.
var tabBarInactiveStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6C7086")).
	Padding(0, 1)

// tabActiveStyle returns the active-tab style: kind tabs use their pill
// colors, TabAll uses the neutral title style.
func tabActiveStyle(t TabKind) lipgloss.Style {
	kind, ok := t.componentKind()
	if !ok {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(viewtui.TitleForeground)).
			Background(lipgloss.Color(viewtui.TitleBackground)).
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
func RenderTabBar(active TabKind, loading bool) string {
	var parts []string

	for i := range int(numTabs) {
		t := TabKind(i)
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
