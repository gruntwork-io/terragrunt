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
	// Module type pill (green-tinted).
	modulePillBg  = "#2B3D2B"
	modulePillFg  = "#A6E3A1"
	modulePillBgS = "#3D5A3D"
	modulePillFgS = "#C7F5C9"

	// Stack type pill (blue-tinted).
	stackPillBg  = "#2B2F3D"
	stackPillFg  = "#89B4FA"
	stackPillBgS = "#3D4460"
	stackPillFgS = "#B4DAFF"

	// Version pill (neutral).
	versionBg  = "#313244"
	versionFg  = "#BAC2DE"
	versionBgS = "#45475A"
	versionFgS = "#CDD6F4"

	// Source URL (muted).
	sourceColor  = "#7F849C"
	sourceColorS = "#9399B2"

	// Description (blue/link-like, prominent).
	descForegroundColor = "#89B4FA"

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
func (d catalogDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) { //nolint:funlen,cyclop,gocritic // value receiver required by list.ItemDelegate interface
	entry, isEntry := item.(*ModuleEntry)
	if !isEntry {
		return
	}

	if m.Width() <= 0 {
		return
	}

	s := &d.styles

	title := entry.Title()
	desc := entry.Description()

	textwidth := m.Width() - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()
	title = ansi.Truncate(title, textwidth, "…")

	// Limit description to a single line
	var lines []string

	for i, line := range strings.Split(desc, "\n") {
		if i >= 1 {
			break
		}

		lines = append(lines, ansi.Truncate(line, textwidth, "…"))
	}

	desc = strings.Join(lines, "\n")

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	isFiltered := m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied

	var matchedRunes []int

	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	// Determine padding for the metadata row alignment
	var padL, padR int

	switch {
	case emptyFilter:
		padL = s.DimmedTitle.GetPaddingLeft()
		padR = s.DimmedTitle.GetPaddingRight()
	case isSelected && m.FilterState() != list.Filtering:
		padL = s.SelectedTitle.GetPaddingLeft()
		padR = s.SelectedTitle.GetPaddingRight()
		// Account for the border on the selected style
		padL += s.SelectedTitle.GetBorderLeftSize()
	default:
		padL = s.NormalTitle.GetPaddingLeft()
		padR = s.NormalTitle.GetPaddingRight()
	}

	// Style title and description based on state
	switch {
	case emptyFilter:
		title = s.DimmedTitle.Render(title)
		desc = s.DimmedDesc.Render(desc)
	case isSelected && m.FilterState() != list.Filtering:
		if isFiltered {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}

		title = s.SelectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	default:
		if isFiltered {
			unmatched := s.NormalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}

		title = s.NormalTitle.Render(title)
		desc = s.NormalDesc.Render(desc)
	}

	// Build metadata row
	metaInnerWidth := m.Width() - padL - padR
	if metaInnerWidth < 1 {
		metaInnerWidth = 1
	}

	selected := isSelected && m.FilterState() != list.Filtering
	colors := metaPalette(entry.ItemType, selected, emptyFilter)
	metaContent := buildMetaRow(entry, metaInnerWidth, &colors)

	var metaLine string

	if selected {
		// Derive from SelectedTitle so the border type/color/padding match exactly,
		// but clear the text foreground so pill colors show through.
		metaLine = s.SelectedTitle.
			Foreground(lipgloss.NoColor{}).
			Render(metaContent)
	} else {
		metaLine = lipgloss.NewStyle().Padding(0, padR, 0, padL).Render(metaContent)
	}

	// Order: title, description, metadata.
	fmt.Fprintf(w, "%s\n%s\n%s", title, desc, metaLine) //nolint:errcheck,gosec
}

// metaPalette returns pill/text styles for the metadata row based on item type and selection state.
func metaPalette(itemType string, selected, dimmed bool) catalogMetaColors {
	if dimmed {
		muted := lipgloss.Color(metaMuted)

		return catalogMetaColors{
			typePill:    lipgloss.NewStyle().Foreground(muted),
			source:      lipgloss.NewStyle().Foreground(muted),
			versionPill: lipgloss.NewStyle().Foreground(muted),
		}
	}

	// Pick type-pill colors based on item type (module=green, stack=blue).
	pillBg, pillFg := modulePillBg, modulePillFg
	pillBgSel, pillFgSel := modulePillBgS, modulePillFgS

	if itemType == "stack" {
		pillBg, pillFg = stackPillBg, stackPillFg
		pillBgSel, pillFgSel = stackPillBgS, stackPillFgS
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

	selectedDescForegroundColorDark       = "#89B4FA"
	selectedDescBorderForegroundColorDark = "#63C5DA"
)
