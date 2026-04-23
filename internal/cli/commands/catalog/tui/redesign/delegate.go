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

	// Unit type pill (blue, matching the `list` / `find` command color).
	unitPillBg  = "#1B46DD"
	unitPillFg  = "#FFFFFF"
	unitPillBgS = "#2E5BEA"
	unitPillFgS = "#FFFFFF"

	// Stack type pill (green, matching the `list` / `find` command color).
	stackPillBg  = "#2E8B57"
	stackPillFg  = "#FFFFFF"
	stackPillBgS = "#3CA068"
	stackPillFgS = "#FFFFFF"

	// Version pill (neutral).
	versionBg  = "#313244"
	versionFg  = "#BAC2DE"
	versionBgS = "#45475A"
	versionFgS = "#CDD6F4"

	// Source URL (neutral gray, matching the help/controls bar tone).
	sourceColor  = "#4A4A4A"
	sourceColorS = "#5A5A5A"

	// Description (muted blue-gray, readable but secondary to title).
	descForegroundColor = "#8393A7"

	metaMuted = "#6C7086"

	// delegateHeight is the number of lines per catalog item (title + desc + meta).
	delegateHeight = 3
)

// catalogDelegate renders catalog modules with a color-coded metadata row
// (type pill, source, version pill) below the title and description.
type catalogDelegate struct {
	styles    list.DefaultItemStyles
	keys      *tui.DelegateKeyMap
	shortHelp func() []key.Binding
	fullHelp  func() [][]key.Binding
}

func newCatalogDelegate(keys *tui.DelegateKeyMap) catalogDelegate {
	// Use the same default item styles as the production delegate, with our color overrides.
	styles := list.NewDefaultItemStyles(true)

	styles.SelectedTitle = styles.SelectedTitle.
		Foreground(lipgloss.Color(selectedTitleForegroundColorDark)).
		BorderForeground(lipgloss.Color(selectedTitleBorderForegroundColorDark))

	styles.NormalDesc = styles.NormalDesc.
		Foreground(lipgloss.Color(descForegroundColor))

	styles.SelectedDesc = styles.SelectedTitle.
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
	}
}

// Height returns the delegate's preferred height (title + desc + meta + spacing).
func (d catalogDelegate) Height() int { //nolint:gocritic // value receiver required by list.ItemDelegate interface
	return delegateHeight
}

// Spacing returns the gap between items.
func (d catalogDelegate) Spacing() int { //nolint:gocritic // value receiver required by list.ItemDelegate interface
	return 1
}

// Update is a no-op; input is handled by the model.
func (d catalogDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { //nolint:gocritic // value receiver required by list.ItemDelegate interface
	return nil
}

// ShortHelp returns the delegate's short help bindings.
func (d catalogDelegate) ShortHelp() []key.Binding { //nolint:gocritic // value receiver required by list.ItemDelegate interface
	if d.shortHelp != nil {
		return d.shortHelp()
	}

	return nil
}

// FullHelp returns the delegate's full help bindings.
func (d catalogDelegate) FullHelp() [][]key.Binding { //nolint:gocritic // value receiver required by list.ItemDelegate interface
	if d.fullHelp != nil {
		return d.fullHelp()
	}

	return nil
}

// Render prints an item with title, description, and metadata row.
func (d catalogDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) { //nolint:gocritic // value receiver required by list.ItemDelegate interface
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
	colors := metaPalette(entry.Kind(), selected, emptyFilter)
	metaContent := buildMetaRow(entry, metaInnerWidth, &colors)
	metaLine := styleMetaLine(s, metaContent, selected, padL, padR)

	fmt.Fprintf(w, "%s\n%s\n%s", title, desc, metaLine) //nolint:errcheck,gosec
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

	selectedDescForegroundColorDark       = "#59788E"
	selectedDescBorderForegroundColorDark = "#63C5DA"
)
