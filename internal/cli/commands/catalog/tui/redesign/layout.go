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
	leftW := avail / 2
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

// buildMetaRow returns one lipgloss-rendered row: [type]  source  [version].
// innerWidth is the available width for the metadata content (excluding title padding).
func buildMetaRow(entry *ModuleEntry, innerWidth int, colors catalogMetaColors) string {
	if entry == nil {
		return ""
	}

	gap := strings.Repeat(" ", metaGap)
	gapW := metaGap

	// Render type pill
	var typePart string
	var typeW int

	if entry.ItemType != "" {
		typePart = colors.typePill.Render(entry.ItemType)
		typeW = lipgloss.Width(typePart)
	}

	// Render version pill
	var verPart string
	var verW int

	if entry.Version != "" {
		verDisplay := entry.Version
		if lipgloss.Width(verDisplay) > maxVerWidth {
			verDisplay = ansi.Truncate(verDisplay, maxVerWidth, "…")
		}

		verPart = colors.versionPill.Render(verDisplay)
		verW = lipgloss.Width(verPart)
	}

	// Calculate source column width
	var srcPart string

	if entry.Source != "" {
		usedWidth := typeW + verW
		gaps := 0

		if typeW > 0 {
			gaps++
		}

		if verW > 0 {
			gaps++
		}

		usedWidth += gaps * gapW
		srcMax := innerWidth - usedWidth

		if srcMax >= minSourceWidth {
			srcDisplay := abbreviateMiddle(entry.Source, srcMax)
			srcPart = colors.source.Render(srcDisplay)
		}
	}

	// Assemble left side (type + source)
	var leftParts []string

	if typePart != "" {
		leftParts = append(leftParts, typePart)
	}

	if srcPart != "" {
		leftParts = append(leftParts, srcPart)
	}

	if len(leftParts) == 0 && verPart == "" {
		return ""
	}

	left := strings.Join(leftParts, gap)

	// Right-align version pill by filling remaining space
	if verPart == "" {
		return left
	}

	leftW := lipgloss.Width(left)
	fill := innerWidth - leftW - verW
	if fill < metaGap {
		fill = metaGap
	}

	return left + strings.Repeat(" ", fill) + verPart
}
