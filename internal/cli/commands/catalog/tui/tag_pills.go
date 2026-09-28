package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
)

// KindForTag returns the component.Kind that tag names case-insensitively,
// and a bool reporting whether tag matched any known kind.
func KindForTag(tag string) (component.Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "module":
		return component.KindModule, true
	case "template":
		return component.KindTemplate, true
	case "unit":
		return component.KindUnit, true
	case "stack":
		return component.KindStack, true
	}

	return 0, false
}

// TagPillStyle returns the pill style for a tag. Privileged tags reuse the
// kind palette; neutral tags use the dimmer tag palette so the row reads
// tertiary.
func TagPillStyle(tag string, selected bool) lipgloss.Style {
	if kind, ok := KindForTag(tag); ok {
		bg, fg := pillColorsForKind(kind, selected)

		return lipgloss.NewStyle().
			Background(lipgloss.Color(bg)).
			Foreground(lipgloss.Color(fg)).
			Padding(0, 1)
	}

	bg, fg := tagBg, tagFg
	if selected {
		bg, fg = tagBgS, tagFgS
	}

	return lipgloss.NewStyle().
		Background(lipgloss.Color(bg)).
		Foreground(lipgloss.Color(fg)).
		Padding(0, 1)
}

// pillColorsForKind returns the (bg, fg) hex pair for a kind's pill.
func pillColorsForKind(kind component.Kind, selected bool) (string, string) {
	switch kind {
	case component.KindTemplate:
		if selected {
			return templatePillBgS, templatePillFgS
		}

		return templatePillBg, templatePillFg
	case component.KindUnit:
		if selected {
			return unitPillBgS, unitPillFgS
		}

		return unitPillBg, unitPillFg
	case component.KindStack:
		if selected {
			return stackPillBgS, stackPillFgS
		}

		return stackPillBg, stackPillFg
	case component.KindModule:
		if selected {
			return modulePillBgS, modulePillFgS
		}

		return modulePillBg, modulePillFg
	}

	if selected {
		return modulePillBgS, modulePillFgS
	}

	return modulePillBg, modulePillFg
}

// tagsLabelText is the prefix shown ahead of tag pills so users can tell
// at a glance that the row is a tag list and not just unrelated metadata.
const tagsLabelText = "tags: "

// tagsLabelStyle returns the muted lipgloss style used to render the
// `tags: ` prefix.
func tagsLabelStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(metaMuted))
}

// RenderTagPills renders tags as space-joined pills under a `tags: ` label,
// fit within maxWidth with a trailing `+N` pill for any that don't fit.
// Privileged tags sort first.
func RenderTagPills(tags []string, maxWidth int, selected bool) string {
	if len(tags) == 0 || maxWidth <= 0 {
		return ""
	}

	const sep = " "

	sepW := lipgloss.Width(sep)

	label := tagsLabelStyle().Render(tagsLabelText)
	labelW := lipgloss.Width(label)

	if labelW >= maxWidth {
		return ""
	}

	sorted := sortPrivilegedFirst(sanitizeTags(tags))

	// Reserve against the widest possible +N so the indicator always fits.
	worstOverflowText := fmt.Sprintf("+%d", len(sorted))
	worstOverflowW := lipgloss.Width(
		TagPillStyle(worstOverflowText, selected).Render(worstOverflowText),
	)

	budget := maxWidth - labelW

	var (
		rendered []string
		used     int
	)

	for i, tag := range sorted {
		pill := TagPillStyle(tag, selected).Render(tag)
		w := lipgloss.Width(pill)

		extra := w
		if len(rendered) > 0 {
			extra += sepW
		}

		// Reserve room for the +N pill if more tags remain.
		reserve := 0
		if i < len(sorted)-1 {
			reserve = sepW + worstOverflowW
		}

		if used+extra+reserve > budget {
			break
		}

		rendered = append(rendered, pill)
		used += extra
	}

	if len(rendered) < len(sorted) {
		remaining := len(sorted) - len(rendered)
		text := fmt.Sprintf("+%d", remaining)
		overflowPill := TagPillStyle(text, selected).Render(text)
		overflowW := lipgloss.Width(overflowPill)

		switch {
		case len(rendered) > 0 && used+sepW+overflowW <= budget:
			rendered = append(rendered, overflowPill)
		case len(rendered) == 0 && overflowW <= budget:
			rendered = append(rendered, overflowPill)
		}
	}

	if len(rendered) == 0 {
		return ""
	}

	return label + strings.Join(rendered, sep)
}

// RenderDetailTagPills renders all tags as pills, no width cap, for the
// detail view above the README. Privileged tags sort first.
func RenderDetailTagPills(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	sorted := sortPrivilegedFirst(sanitizeTags(tags))
	rendered := make([]string, 0, len(sorted))

	for _, tag := range sorted {
		rendered = append(rendered, TagPillStyle(tag, false).Render(tag))
	}

	return tagsLabelStyle().Render(tagsLabelText) + strings.Join(rendered, " ")
}

// TagsMarkdownSection returns a `## Tags` markdown block for appending to a
// README. Privileged tags sort first.
func TagsMarkdownSection(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	sorted := sortPrivilegedFirst(tags)

	var b strings.Builder

	b.WriteString("\n\n## Tags\n\n")

	for _, tag := range sorted {
		b.WriteString("- ")
		b.WriteString(tag)
		b.WriteByte('\n')
	}

	return b.String()
}

// sanitizeTags returns tags ready to draw. A pill is measured before it is
// placed, so the text is sanitized before any width is taken from it.
func sanitizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		out = append(out, viewtui.SanitizeLabel(tag))
	}

	return out
}

// sortPrivilegedFirst returns tags reordered so kind-matching ones come
// first, preserving authoring order within each group.
func sortPrivilegedFirst(tags []string) []string {
	out := slices.Clone(tags)

	slices.SortStableFunc(out, func(a, b string) int {
		_, aPriv := KindForTag(a)
		_, bPriv := KindForTag(b)

		switch {
		case aPriv && !bPriv:
			return -1
		case !aPriv && bPriv:
			return 1
		}

		return 0
	})

	return out
}
