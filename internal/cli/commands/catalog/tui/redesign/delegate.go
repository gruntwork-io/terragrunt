package redesign

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// Pill and metadata color constants.
const (
	// Module type pill (OpenTofu yellow).
	modulePillBg  = "#3D3520"
	modulePillFg  = "#FFDA18"
	modulePillBgS = "#4D4328"
	modulePillFgS = "#FFE44D"

	// Template type pill (mauve).
	templatePillBg  = "#2A2040"
	templatePillFg  = "#CBA6F7"
	templatePillBgS = "#3A2D55"
	templatePillFgS = "#DDC4FA"

	// Unit type pill (blue).
	unitPillBg  = "#1E2840"
	unitPillFg  = "#89B4FA"
	unitPillBgS = "#2A3855"
	unitPillFgS = "#ABCAFF"

	// Stack type pill (green).
	stackPillBg  = "#1F2D20"
	stackPillFg  = "#A6E3A1"
	stackPillBgS = "#2C402D"
	stackPillFgS = "#C5EBC1"

	// Version pill (neutral).
	versionBg  = "#313244"
	versionFg  = "#BAC2DE"
	versionBgS = "#45475A"
	versionFgS = "#CDD6F4"

	// Neutral tag pill (tertiary; reads dimmer than the description so the
	// tag row recedes behind the title and description).
	tagBg  = "#21252F"
	tagFg  = "#6C7388"
	tagBgS = "#2A2E3B"
	tagFgS = "#828A9E"

	// Source URL (neutral gray, matching the help/controls bar tone).
	sourceColor  = "#4A4A4A"
	sourceColorS = "#5A5A5A"

	// Description (blue-gray; bold titles carry the hierarchy so this can read
	// brighter than a typical "secondary" color without competing).
	descForegroundColor = "#B0BFD0"

	metaMuted = "#6C7086"

	// delegateHeight is the number of lines per catalog item (title + desc + meta).
	delegateHeight = 3
	// delegateHeightWithTagsRow is the height when tags occupy their own line.
	delegateHeightWithTagsRow = 4
)

// catalogDelegate renders catalog modules with a color-coded metadata row
// (type pill, source, version pill) below the title and description.
type catalogDelegate struct {
	styles     list.DefaultItemStyles
	keys       *tui.DelegateKeyMap
	shortHelp  func() []key.Binding
	fullHelp   func() [][]key.Binding
	tagsLayout tagsListLayout
}

func newCatalogDelegate(env map[string]string, keys *tui.DelegateKeyMap) catalogDelegate {
	// Use the same default item styles as the production delegate, with our color overrides.
	styles := list.NewDefaultItemStyles(true)

	styles.NormalTitle = styles.NormalTitle.Bold(true)

	styles.SelectedTitle = styles.SelectedTitle.
		Bold(true).
		Foreground(lipgloss.Color(selectedTitleForegroundColorDark)).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color(selectedTitleBorderForegroundColorDark))

	styles.NormalDesc = styles.NormalDesc.
		Foreground(lipgloss.Color(descForegroundColor))

	styles.SelectedDesc = styles.SelectedTitle.
		Bold(false).
		Foreground(lipgloss.Color(selectedDescForegroundColorDark)).
		BorderForeground(lipgloss.Color(selectedDescBorderForegroundColorDark))

	help := []key.Binding{keys.Choose, keys.Scaffold}

	return catalogDelegate{
		styles: styles,
		keys:   keys,
		shortHelp: func() []key.Binding {
			return help
		},
		fullHelp: func() [][]key.Binding {
			return [][]key.Binding{help}
		},
		tagsLayout: resolveTagsListLayout(env),
	}
}

// Height returns the delegate's preferred height. In the default meta layout
// each item is title + desc + meta (3 lines); in the row layout tags get
// their own fourth line.
func (d catalogDelegate) Height() int {
	if d.tagsLayout == tagsListLayoutRow {
		return delegateHeightWithTagsRow
	}

	return delegateHeight
}

// Spacing returns the gap between items.
func (d catalogDelegate) Spacing() int {
	return 1
}

// Update is a no-op; input is handled by the model.
func (d catalogDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// ShortHelp returns the delegate's short help bindings.
func (d catalogDelegate) ShortHelp() []key.Binding {
	if d.shortHelp != nil {
		return d.shortHelp()
	}

	return nil
}

// FullHelp returns the delegate's full help bindings.
func (d catalogDelegate) FullHelp() [][]key.Binding {
	if d.fullHelp != nil {
		return d.fullHelp()
	}

	return nil
}

// Render prints an item with title, description, and metadata row.
func (d catalogDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, isEntry := item.(*ComponentEntry)
	if !isEntry {
		return
	}

	if m.Width() <= 0 {
		return
	}

	s := &d.styles

	textwidth := m.Width() - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()
	title := ansi.Truncate(entry.Title(), textwidth, "…")
	desc := truncateFirstLine(entry.Description(), textwidth)

	selected := index == m.Index() && m.FilterState() != list.Filtering
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	isFiltered := m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied

	var matchedRunes []int
	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	padL, padR := metaPadding(s, selected, emptyFilter)
	title, desc = styleTitleDesc(s, title, desc, selected, emptyFilter, isFiltered, matchedRunes)

	metaInnerWidth := max(1, m.Width()-padL-padR)

	includeTagsInMeta := d.tagsLayout == tagsListLayoutMeta && !emptyFilter
	metaContent := BuildMetaRow(entry, metaInnerWidth, includeTagsInMeta, selected, emptyFilter)
	metaLine := styleMetaLine(s, metaContent, selected, padL, padR)

	if d.tagsLayout != tagsListLayoutRow {
		fmt.Fprintf(w, "%s\n%s\n%s", title, desc, metaLine) //nolint:errcheck,gosec

		return
	}

	tagsContent := ""
	if !emptyFilter {
		tagsContent = renderTagPills(entry.Tags(), metaInnerWidth, selected)
	}

	tagsLine := styleMetaLine(s, tagsContent, selected, padL, padR)

	fmt.Fprintf(w, "%s\n%s\n%s\n%s", title, desc, metaLine, tagsLine) //nolint:errcheck,gosec
}

// truncateFirstLine returns only the first line of s, truncated to maxWidth.
func truncateFirstLine(s string, maxWidth int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}

	return ansi.Truncate(s, maxWidth, "…")
}

// metaPadding returns left/right padding for the metadata row based on
// the current selection and filter state.
func metaPadding(s *list.DefaultItemStyles, selected, emptyFilter bool) (int, int) {
	if emptyFilter {
		return s.DimmedTitle.GetPaddingLeft(), s.DimmedTitle.GetPaddingRight()
	}

	if selected {
		padL := s.SelectedTitle.GetPaddingLeft() + s.SelectedTitle.GetBorderLeftSize()
		return padL, s.SelectedTitle.GetPaddingRight()
	}

	return s.NormalTitle.GetPaddingLeft(), s.NormalTitle.GetPaddingRight()
}

// styleTitleDesc applies the appropriate lipgloss styles to the title and
// description strings based on selection and filter state.
func styleTitleDesc(
	s *list.DefaultItemStyles,
	title, desc string,
	selected, emptyFilter, isFiltered bool,
	matchedRunes []int,
) (string, string) {
	if emptyFilter {
		return s.DimmedTitle.Render(title), s.DimmedDesc.Render(desc)
	}

	if selected {
		if isFiltered {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}

		return s.SelectedTitle.Render(title), s.SelectedDesc.Render(desc)
	}

	if isFiltered {
		unmatched := s.NormalTitle.Inline(true)
		matched := unmatched.Inherit(s.FilterMatch)
		title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
	}

	return s.NormalTitle.Render(title), s.NormalDesc.Render(desc)
}

// styleMetaLine wraps the metadata content in the appropriate style. Selected
// items inherit from SelectedTitle so the left border aligns with the title.
func styleMetaLine(s *list.DefaultItemStyles, content string, selected bool, padL, padR int) string {
	if selected {
		return s.SelectedTitle.
			Foreground(lipgloss.NoColor{}).
			Render(content)
	}

	return lipgloss.NewStyle().Padding(0, padR, 0, padL).Render(content)
}

// metaPalette returns pill/text styles for the metadata row based on component kind and selection state.
func metaPalette(kind ComponentKind, selected, dimmed bool) catalogMetaColors {
	if dimmed {
		muted := lipgloss.Color(metaMuted)

		return catalogMetaColors{
			typePill:    lipgloss.NewStyle().Foreground(muted),
			source:      lipgloss.NewStyle().Foreground(muted),
			versionPill: lipgloss.NewStyle().Foreground(muted),
		}
	}

	// Pick type-pill colors based on component kind.
	pillBg, pillFg := modulePillBg, modulePillFg
	pillBgSel, pillFgSel := modulePillBgS, modulePillFgS

	switch kind {
	case ComponentKindTemplate:
		pillBg, pillFg = templatePillBg, templatePillFg
		pillBgSel, pillFgSel = templatePillBgS, templatePillFgS
	case ComponentKindUnit:
		pillBg, pillFg = unitPillBg, unitPillFg
		pillBgSel, pillFgSel = unitPillBgS, unitPillFgS
	case ComponentKindStack:
		pillBg, pillFg = stackPillBg, stackPillFg
		pillBgSel, pillFgSel = stackPillBgS, stackPillFgS
	case ComponentKindModule:
		// Defaults already applied above.
	}

	if selected {
		return catalogMetaColors{
			typePill: lipgloss.NewStyle().
				Background(lipgloss.Color(pillBgSel)).
				Foreground(lipgloss.Color(pillFgSel)).
				Padding(0, 1),
			source: lipgloss.NewStyle().
				Foreground(lipgloss.Color(sourceColorS)),
			versionPill: lipgloss.NewStyle().
				Background(lipgloss.Color(versionBgS)).
				Foreground(lipgloss.Color(versionFgS)).
				Padding(0, 1),
		}
	}

	return catalogMetaColors{
		typePill: lipgloss.NewStyle().
			Background(lipgloss.Color(pillBg)).
			Foreground(lipgloss.Color(pillFg)).
			Padding(0, 1),
		source: lipgloss.NewStyle().
			Foreground(lipgloss.Color(sourceColor)),
		versionPill: lipgloss.NewStyle().
			Background(lipgloss.Color(versionBg)).
			Foreground(lipgloss.Color(versionFg)).
			Padding(0, 1),
	}
}

// Color constants from the production delegate, reused here for consistency.
const (
	selectedTitleForegroundColorDark       = "#63C5DA"
	selectedTitleBorderForegroundColorDark = "#63C5DA"

	selectedDescForegroundColorDark       = "#8AA3B5"
	selectedDescBorderForegroundColorDark = "#63C5DA"
)
