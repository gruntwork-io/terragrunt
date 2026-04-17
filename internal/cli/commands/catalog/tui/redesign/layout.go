package redesign

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxVerWidth    = 22
	metaGap        = 2
	minSourceWidth = 8
	halveDivisor   = 2
)

// catalogMetaColors holds per-column lipgloss styles for the catalog metadata row.
type catalogMetaColors struct {
	typePill    lipgloss.Style
	source      lipgloss.Style
	versionPill lipgloss.Style
}

// abbreviateMiddle shortens s to maxWidth cells with an ellipsis in the middle,
// preserving the prefix (e.g. domain) and suffix (e.g. repo name).
func abbreviateMiddle(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	w := lipgloss.Width(s)
	if w <= maxWidth {
		return s
	}

	const ell = "…"

	ellW := lipgloss.Width(ell)
	if maxWidth <= ellW {
		return ansi.Truncate(s, maxWidth, "")
	}

	avail := maxWidth - ellW
	leftW := avail / halveDivisor
	rightW := avail - leftW

	return takeWidthPrefix(s, leftW) + ell + takeWidthSuffix(s, rightW)
}

func takeWidthPrefix(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}

	var b strings.Builder

	w := 0

	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > maxW {
			break
		}

		b.WriteRune(r)

		w += rw
	}

	return b.String()
}

func takeWidthSuffix(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}

	rs := []rune(s)

	var parts []rune

	w := 0

	for i := len(rs) - 1; i >= 0; i-- {
		r := rs[i]
		rw := lipgloss.Width(string(r))

		if w+rw > maxW {
			break
		}

		parts = append(parts, r)
		w += rw
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return string(parts)
}

// buildMetaRow returns one lipgloss-rendered row: [type] [version] source.
// All parts are left-aligned. innerWidth is the available width (excluding padding).
func buildMetaRow(entry *ModuleEntry, innerWidth int, colors *catalogMetaColors) string {
	if entry == nil {
		return ""
	}

	gap := strings.Repeat(" ", metaGap)

	var parts []string

	usedWidth := 0

	// Type pill.
	if entry.ItemType != "" {
		part := colors.typePill.Render(entry.ItemType)
		parts = append(parts, part)
		usedWidth += lipgloss.Width(part)
	}

	// Version pill (inline, right after type).
	if entry.Version != "" {
		verDisplay := entry.Version
		if lipgloss.Width(verDisplay) > maxVerWidth {
			verDisplay = ansi.Truncate(verDisplay, maxVerWidth, "…")
		}

		part := colors.versionPill.Render(verDisplay)
		parts = append(parts, part)
		usedWidth += lipgloss.Width(part)
	}

	// Source URL (gets remaining width).
	if entry.Source != "" {
		// Gaps between all parts: if we add source, total gaps = len(parts).
		srcMax := innerWidth - usedWidth - len(parts)*metaGap

		if srcMax >= minSourceWidth {
			srcDisplay := abbreviateMiddle(entry.Source, srcMax)
			part := colors.source.Render(srcDisplay)
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, gap)
}
