package redesign

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxVerWidth    = 22
	metaGap        = 2
	minSourceWidth = 8

	// sourceColumnWidth caps and right-pads the source URL when tags follow
	// so tag pills land at the same column across rows.
	sourceColumnWidth = 32
)

// kindColumnWidth is the widest type pill across all kinds, used to right-pad
// the type pill so subsequent columns align across rows.
var kindColumnWidth = computeKindColumnWidth()

// computeKindColumnWidth returns the widest rendered type pill.
func computeKindColumnWidth() int {
	style := lipgloss.NewStyle().Padding(0, 1)

	var maxW int

	for _, k := range []ComponentKind{
		ComponentKindModule,
		ComponentKindTemplate,
		ComponentKindUnit,
		ComponentKindStack,
	} {
		if w := lipgloss.Width(style.Render(k.String())); w > maxW {
			maxW = w
		}
	}

	return maxW
}

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
	leftW := avail / 2 //nolint:mnd
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

	slices.Reverse(parts)

	return string(parts)
}

// BuildMetaRow renders the catalog list metadata row: type, version, source,
// and optional tags within innerWidth.
func BuildMetaRow(entry *ComponentEntry, innerWidth int, includeTags, selected, dimmed bool) string {
	if entry == nil {
		return ""
	}

	colors := metaPalette(entry.Kind(), selected, dimmed)

	gap := strings.Repeat(" ", metaGap)

	var parts []string

	usedWidth := 0

	remaining := func() int {
		return innerWidth - usedWidth - len(parts)*metaGap
	}

	// Pad the kind column when more content follows, so columns align.
	hasMore := entry.Version != "" || entry.Source != "" || (includeTags && len(entry.Tags()) > 0)

	// Type pill.
	kindLabel := entry.Kind().String()
	if kindLabel != "" {
		pill := colors.typePill.Render(kindLabel)
		pillW := lipgloss.Width(pill)

		if pillW <= remaining() {
			part := pill
			partW := pillW

			if hasMore && pillW < kindColumnWidth {
				part = pill + strings.Repeat(" ", kindColumnWidth-pillW)
				partW = kindColumnWidth
			}

			parts = append(parts, part)
			usedWidth += partW
		}
	}

	// Version pill (truncate or skip if it won't fit).
	if entry.Version != "" {
		verDisplay := entry.Version
		if lipgloss.Width(verDisplay) > maxVerWidth {
			verDisplay = ansi.Truncate(verDisplay, maxVerWidth, "…")
		}

		part := colors.versionPill.Render(verDisplay)
		partW := lipgloss.Width(part)

		if partW <= remaining() {
			parts = append(parts, part)
			usedWidth += partW
		}
	}

	// Source URL (gets remaining width, skip if too narrow).
	if entry.Source != "" {
		srcMax := remaining()

		tagsFollow := includeTags && len(entry.Tags()) > 0

		// Reserve room for tags, then cap source to align tag pills.
		if tagsFollow {
			srcMax -= metaGap + minSourceWidth
			if srcMax > sourceColumnWidth {
				srcMax = sourceColumnWidth
			}
		}

		if srcMax >= minSourceWidth {
			srcDisplay := abbreviateMiddle(entry.Source, srcMax)
			srcW := lipgloss.Width(srcDisplay)

			var (
				part  string
				partW int
			)

			if tagsFollow && srcW < srcMax {
				// Pad outside the colored span so trailing spaces stay blank.
				part = colors.source.Render(srcDisplay) + strings.Repeat(" ", srcMax-srcW)
				partW = srcMax
			} else {
				part = colors.source.Render(srcDisplay)
				partW = srcW
			}

			parts = append(parts, part)
			usedWidth += partW
		}
	}

	if includeTags {
		tagsBudget := remaining()
		if tagsBudget > 0 {
			if tagsLine := renderTagPills(entry.Tags(), tagsBudget, selected); tagsLine != "" {
				parts = append(parts, tagsLine)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, gap)
}
