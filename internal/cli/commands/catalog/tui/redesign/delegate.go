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
	typePillBg  = "#45475A"
	typePillFg  = "#CBA6F7"
	versionBg   = "#313244"
	versionFg   = "#A6E3A1"
	sourceColor = "#89B4FA"

	typePillBgSelected  = "#585B70"
	typePillFgSelected  = "#E4C0FF"
	versionBgSelected   = "#45475A"
	versionFgSelected   = "#C7F5C9"
	sourceColorSelected = "#B4DAFF"

	metaMuted = "#6C7086"
)

// catalogDelegate renders catalog modules with a color-coded metadata row
// (type pill, source, version pill) below the title and description.
type catalogDelegate struct {
	styles     list.DefaultItemStyles
	keys       *tui.DelegateKeyMap
	shortHelp  func() []key.Binding
	fullHelp   func() [][]key.Binding
}

func newCatalogDelegate(keys *tui.DelegateKeyMap) catalogDelegate {
	// Use the same default item styles as the production delegate, with our color overrides.
	styles := list.NewDefaultItemStyles(true)

	styles.SelectedTitle = styles.SelectedTitle.
		Foreground(lipgloss.Color(selectedTitleForegroundColorDark)).
		BorderForeground(lipgloss.Color(selectedTitleBorderForegroundColorDark))

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
func (d catalogDelegate) Height() int {
	return 3
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
func (d catalogDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) { //nolint:funlen,cyclop
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

	colors := metaPalette(isSelected && m.FilterState() != list.Filtering, emptyFilter)
	metaLine := lipgloss.NewStyle().Padding(0, padR, 0, padL).Render(
		buildMetaRow(entry, metaInnerWidth, colors))

	fmt.Fprintf(w, "%s\n%s\n%s", title, desc, metaLine) //nolint:errcheck,gosec
}

// metaPalette returns pill/text styles for the metadata row based on selection state.
func metaPalette(selected, dimmed bool) catalogMetaColors {
	if dimmed {
		muted := lipgloss.Color(metaMuted)

		return catalogMetaColors{
			typePill:    lipgloss.NewStyle().Foreground(muted),
			source:      lipgloss.NewStyle().Foreground(muted),
			versionPill: lipgloss.NewStyle().Foreground(muted),
		}
	}

	if selected {
		return catalogMetaColors{
			typePill: lipgloss.NewStyle().
				Background(lipgloss.Color(typePillBgSelected)).
				Foreground(lipgloss.Color(typePillFgSelected)).
				Padding(0, 1),
			source: lipgloss.NewStyle().
				Foreground(lipgloss.Color(sourceColorSelected)),
			versionPill: lipgloss.NewStyle().
				Background(lipgloss.Color(versionBgSelected)).
				Foreground(lipgloss.Color(versionFgSelected)).
				Padding(0, 1),
		}
	}

	return catalogMetaColors{
		typePill: lipgloss.NewStyle().
			Background(lipgloss.Color(typePillBg)).
			Foreground(lipgloss.Color(typePillFg)).
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
