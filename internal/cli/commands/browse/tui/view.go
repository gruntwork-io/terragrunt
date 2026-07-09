package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gruntwork-io/terragrunt/internal/component"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
)

// columnCount is the number of Miller columns: parent, current, preview.
const columnCount = 3

// paneBorderWidth is the horizontal space a pane's left and right border take.
const paneBorderWidth = 2

// paneBorderHeight is the vertical space a pane's top and bottom border take.
const paneBorderHeight = 2

// panePadWidth is the horizontal space a pane's left and right padding take.
const panePadWidth = 2

const (
	// itemColor renders unselected entries in bright white.
	itemColor = "15"
	// dimColor is used for borders, help text, and empty-state placeholders.
	dimColor = "240"
	// dependencyColor marks entries the highlighted component depends on.
	dependencyColor = "#A6E22E"
	// dependentColor marks entries that depend on the highlighted component.
	dependentColor = "#FD971F"
)

var (
	appStyle      = lipgloss.NewStyle().Padding(1, 2) //nolint:mnd
	itemStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(itemColor))
	selectedStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(viewtui.SelectionText)).
			Background(lipgloss.Color(viewtui.SelectionBlue))
	dependencyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(dependencyColor))
	dependentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(dependentColor))
	valueStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(itemColor))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(dimColor))
	helpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(dimColor)).Padding(1, 0, 0, 0)
	headerStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(viewtui.SelectionBlue)).Padding(0, 1, 1, 1)
)

// paneStyle returns the rounded-border box style for a pane of the given total
// width and height. Lipgloss counts the border inside Width and Height, so
// these are the pane's full footprint on screen.
func paneStyle(paneWidth, paneHeight int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(dimColor)).
		Padding(0, 1).
		Width(paneWidth).
		Height(paneHeight)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}

	header := m.headerView()
	footer := helpStyle.Render(m.footerView())

	sideWidth, previewWidth, paneHeight := m.paneSizes(header, footer)

	left := paneStyle(sideWidth, paneHeight).Render("")

	if m.current.parent != nil {
		siblings := m.current.parent.children
		left = m.renderColumn(siblings, slices.Index(siblings, m.current), sideWidth, paneHeight, "")
	}

	middle := m.renderColumn(m.current.children, m.cursor[m.current], sideWidth, paneHeight, m.activeQuery())
	right := m.renderPreview(previewWidth, paneHeight)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
	body := lipgloss.JoinVertical(lipgloss.Left, header, columns, footer)

	content := m.toasts.Overlay(appStyle.Render(body), m.width, m.height)

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

// paneSizes returns the total width of a side pane (parent or current), the
// total width of the preview pane, and the shared total pane height, borders
// included, all derived from the current terminal size. The preview pane gets
// half the space; the parent and current panes share the rest equally. Fixed
// ratios keep the layout stable as you navigate. Dimensions are clamped to
// zero so a tiny terminal degrades to empty panes instead of garbled output.
// The rendered header and footer are passed in so a single View() renders them
// once and reuses them for both measurement and display.
func (m Model) paneSizes(header, footer string) (sideWidth, previewWidth, paneHeight int) {
	frameH, frameV := appStyle.GetFrameSize()

	content := max(m.width-frameH, 0)
	sideWidth = content / 4 //nolint:mnd
	previewWidth = content - sideWidth*(columnCount-1)

	paneHeight = max(m.height-frameV-lipgloss.Height(header)-lipgloss.Height(footer), 0)

	return sideWidth, previewWidth, paneHeight
}

// measuredPaneSizes renders the header and footer solely to measure the pane
// height, for the Update-path callers that need sizes outside a View() render.
func (m Model) measuredPaneSizes() (sideWidth, previewWidth, paneHeight int) {
	return m.paneSizes(m.headerView(), helpStyle.Render(m.footerView()))
}

// paneInterior returns the text area inside a pane of the given total size,
// once its border and padding are removed, clamped to zero.
func paneInterior(paneWidth, paneHeight int) (width, height int) {
	return max(paneWidth-paneBorderWidth-panePadWidth, 0), max(paneHeight-paneBorderHeight, 0)
}

// previewArea returns the interior width and height of the preview pane: the
// space left for content once the pane's border and padding are removed.
func (m Model) previewArea() (width, height int) {
	_, previewWidth, paneHeight := m.measuredPaneSizes()

	return paneInterior(previewWidth, paneHeight)
}

// pageSize returns how many rows a column shows at once: the distance the
// pgup/pgdown keys move the cursor. At least 1, so paging still moves on a
// terminal too short to fit a single row.
func (m Model) pageSize() int {
	sideWidth, _, paneHeight := m.measuredPaneSizes()
	_, rows := paneInterior(sideWidth, paneHeight)

	return max(rows, 1)
}

// renderColumn renders a list of nodes into a bordered, fixed-size pane,
// highlighting the node at cursorIdx. Only a window of rows around the cursor
// is rendered, so directories larger than the pane scroll instead of
// overflowing it. When query is non-empty, every visible row gains a gutter
// marking whether its name matches, so an active search shows its matches at a
// glance.
func (m Model) renderColumn(nodes []*Node, cursorIdx, paneWidth, paneHeight int, query string) string {
	style := paneStyle(paneWidth, paneHeight)
	rowWidth, rows := paneInterior(paneWidth, paneHeight)

	if rowWidth == 0 || rows == 0 {
		return style.Render("")
	}

	if len(nodes) == 0 {
		return style.Render(ansi.Truncate(dimStyle.Render("(empty)"), rowWidth, ""))
	}

	// The window centers on the cursor and clamps to the list's ends, computed
	// fresh each render so no scroll state can drift.
	offset := min(max(cursorIdx-rows/2, 0), max(len(nodes)-rows, 0))
	visible := nodes[offset:min(offset+rows, len(nodes))]

	searching := query != ""

	lines := make([]string, len(visible))
	for i, n := range visible {
		gutter := gutterNone
		if searching {
			gutter = gutterMiss
			if nameMatches(n.name, query) {
				gutter = gutterHit
			}
		}

		sel := unselected
		if offset+i == cursorIdx {
			sel = selected
		}

		lines[i] = ansi.Truncate(m.renderName(n, sel, gutter, rowWidth), rowWidth, "")
	}

	return style.Render(strings.Join(lines, "\n"))
}

// selection marks whether a rendered row is the highlighted one.
type selection int

const (
	unselected selection = iota
	selected
)

// matchGutter is the search gutter a row carries: none when no search is active,
// or a marker showing whether the row matches the active query.
type matchGutter int

const (
	gutterNone matchGutter = iota
	gutterMiss
	gutterHit
)

// renderName renders a node's label, colored by kind: units blue, stacks green,
// plain directories bold white, and files dimmed. A file a unit reads is shown
// white once discovery reports it, marking it as relevant to the estate.
// Hidden directories are dimmed like files: discovery skips or rarely cares
// about them, so they stay out of the way. Directories carry a trailing slash.
// The highlighted row is a full-width blue bar. While a search is active, the
// gutter reserves space and matched entries are flagged with a marker.
func (m Model) renderName(n *Node, sel selection, gutter matchGutter, rowWidth int) string {
	label := n.name
	if n.kind != KindFile {
		label += "/"
	}

	if gutter != gutterNone {
		marker := "  "
		if gutter == gutterHit {
			marker = "▸ "
		}

		label = marker + label
	}

	switch {
	case sel == selected:
		return selectedStyle.Width(rowWidth).Render(label)
	case n.kind == KindUnit, n.kind == KindStack:
		return m.colorizer.ColorizeKind(label, componentKind(n.kind))
	case n.kind == KindFile:
		if m.disc.isRead(n.absPath) {
			return itemStyle.Render(label)
		}

		return dimStyle.Render(label)
	case strings.HasPrefix(n.name, "."):
		return dimStyle.Render(label)
	default:
		return itemStyle.Render(label)
	}
}

// renderPreview renders the bordered detail pane for the highlighted node:
// metadata for directories and components, a syntax-highlighted preview for
// files.
func (m Model) renderPreview(paneWidth, paneHeight int) string {
	width, height := paneInterior(paneWidth, paneHeight)

	return paneStyle(paneWidth, paneHeight).Render(m.previewContent(m.Selected(), width, height))
}

// headerView renders the path bar: the absolute path of the highlighted entry
// (or the current directory when nothing is highlighted), with the home
// directory abbreviated to ~.
func (m Model) headerView() string {
	target := m.current
	if sel := m.Selected(); sel != nil {
		target = sel
	}

	return headerStyle.Render(abbreviatePath(m.home, target.absPath))
}

// abbreviatePath replaces a leading home directory with ~. An empty home or a
// path outside the home directory is returned unchanged.
func abbreviatePath(home, p string) string {
	if home == "" {
		return p
	}

	if p == home {
		return "~"
	}

	rel, err := filepath.Rel(home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return p
	}

	return filepath.Join("~", rel)
}

// previewContent returns the detail-pane content for the given node: metadata
// for directories and components, and the rendered file preview for files.
// Every variant is clipped to the given interior width and height so tall or
// wide content can't overrun the pane.
func (m Model) previewContent(n *Node, width, height int) string {
	return viewtui.ClipToPane(m.rawPreviewContent(n), width, height)
}

// rawPreviewContent returns the unclipped detail-pane content for a node.
func (m Model) rawPreviewContent(n *Node) string {
	if n == nil {
		return dimStyle.Render("(empty)")
	}

	switch n.kind {
	case KindUnit, KindStack:
		return m.componentPreview(n)
	case KindFile:
		return m.filePreview(n)
	case KindDir:
		return m.dirPreview(n)
	default:
		panic("browse: unhandled node kind " + strconv.Itoa(int(n.kind)))
	}
}

// filePreview returns the cached, syntax-highlighted preview of a file, falling
// back to its dimmed path when nothing has been rendered yet.
func (m Model) filePreview(n *Node) string {
	if n.preview == "" {
		return dimStyle.Render(n.relPath)
	}

	return n.preview
}

// componentPreview renders unit/stack metadata: kind, paths, source, and the
// dependency, dependent, and reading relationships. The kind is known from the
// filesystem classification; the rest waits on discovery and shows a loading
// placeholder until the component resolves.
func (m Model) componentPreview(n *Node) string {
	kind := componentKind(n.kind)

	lines := []string{
		m.field("Kind", m.colorizer.ColorizeKind(string(kind), kind)),
	}

	c := n.component
	if c == nil {
		// After discovery, a component-less unit or stack means discovery excluded
		// it; say so instead of presenting bare metadata-less fields.
		suffix := dimStyle.Render("(not discovered)")
		if !m.disc.done {
			suffix = loadingValue()
		}

		lines = append(lines, "", suffix)

		return strings.Join(lines, "\n")
	}

	if sources := c.Sources(); len(sources) > 0 {
		lines = append(lines, m.field("Source", strings.Join(sources, ", ")))
	}

	lines = append(lines, "")

	if stack, ok := c.(*component.Stack); ok {
		lines = append(lines, m.stackDefinitionLines(stack)...)
	}

	if deps := componentLines(c.Path(), c.Dependencies(), dependencyStyle); len(deps) > 0 {
		lines = append(lines, m.section("Dependencies", deps)...)
	}

	if dependents := componentLines(c.Path(), c.Dependents(), dependentStyle); len(dependents) > 0 {
		lines = append(lines, m.section("Dependents", dependents)...)
	}

	if reading := relativeReadPaths(c); len(reading) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.section("Reading", reading)...)
	}

	return strings.Join(lines, "\n")
}

// relativeReadPaths returns the files a component reads, each made relative to
// the component's own directory.
func relativeReadPaths(c component.Component) []string {
	reading := c.Reading()
	paths := make([]string, 0, len(reading))

	for _, f := range reading {
		paths = append(paths, relTo(c.Path(), f))
	}

	return paths
}

// dirPreview renders a summary of a plain directory: the number of units and
// stacks beneath it, or a loading placeholder until discovery reports them.
func (m Model) dirPreview(n *Node) string {
	if !m.disc.done {
		return strings.Join([]string{
			m.field("Units", loadingValue()),
			m.field("Stacks", loadingValue()),
		}, "\n")
	}

	units, stacks := m.disc.counts(n)

	return strings.Join([]string{
		m.field("Units", strconv.Itoa(units)),
		m.field("Stacks", strconv.Itoa(stacks)),
	}, "\n")
}

// componentKind maps a tree Kind to the component Kind used for coloring and
// labels, defaulting to a unit for anything that isn't a stack.
func componentKind(k Kind) component.Kind {
	if k == KindStack {
		return component.StackKind
	}

	return component.UnitKind
}

// loadingValue is the placeholder shown for a field whose value discovery hasn't
// reported yet.
func loadingValue() string {
	return dimStyle.Render("…")
}

// stackEntry is a unit or stack declared in a terragrunt.stack.hcl file.
type stackEntry struct {
	name   string
	source string
	path   string
}

// stackDefinitionLines lists the units and stacks declared in a stack's
// terragrunt.stack.hcl, each with its source and path. It returns nil when the
// stack config wasn't parsed.
func (m Model) stackDefinitionLines(stack *component.Stack) []string {
	cfg := stack.Config()
	if cfg == nil {
		return nil
	}

	units := make([]stackEntry, 0, len(cfg.Units))
	for _, u := range cfg.Units {
		units = append(units, stackEntry{name: u.Name, source: u.Source, path: u.Path})
	}

	stacks := make([]stackEntry, 0, len(cfg.Stacks))
	for _, s := range cfg.Stacks {
		stacks = append(stacks, stackEntry{name: s.Name, source: s.Source, path: s.Path})
	}

	lines := m.stackEntrySection("Units", units, component.UnitKind)

	return append(lines, m.stackEntrySection("Stacks", stacks, component.StackKind)...)
}

// stackEntrySection renders a heading followed by each entry's name (colored by
// kind) with its source and path nested beneath. It returns nil when empty.
func (m Model) stackEntrySection(label string, entries []stackEntry, kind component.Kind) []string {
	if len(entries) == 0 {
		return nil
	}

	slices.SortFunc(entries, func(a, b stackEntry) int {
		return strings.Compare(a.name, b.name)
	})

	out := []string{m.colorizer.ColorizeHeading(label + ":")}

	for _, e := range entries {
		out = append(out, "  "+m.colorizer.ColorizeKind(e.name, kind))

		if e.source != "" {
			out = append(out, "    "+attr("source", e.source))
		}

		if e.path != "" {
			out = append(out, "    "+attr("path", e.path))
		}
	}

	return out
}

// attr renders a "label: value" line in white for an entry's nested attributes.
func attr(label, value string) string {
	return itemStyle.Render(label+":") + " " + valueStyle.Render(value)
}

// componentLines renders each component as a path relative to base, styled
// with the given style and sorted.
func componentLines(base string, comps component.Components, style lipgloss.Style) []string {
	paths := make([]string, 0, len(comps))
	for _, c := range comps {
		paths = append(paths, relTo(base, c.Path()))
	}

	slices.Sort(paths)

	lines := make([]string, len(paths))
	for i, p := range paths {
		lines[i] = style.Render(p)
	}

	return lines
}

// field renders a single "Label: value" line with a colored heading.
func (m Model) field(label, value string) string {
	return m.colorizer.ColorizeHeading(label+":") + " " + value
}

// section renders a heading followed by indented items. Callers omit the
// section entirely when there are no items.
func (m Model) section(label string, items []string) []string {
	out := make([]string, 0, len(items)+1)
	out = append(out, m.colorizer.ColorizeHeading(label+":"))

	for _, item := range items {
		out = append(out, "  "+item)
	}

	return out
}

// footerView renders the bottom line: the search input while typing, a status
// line summarizing a committed search, and the navigation hint otherwise, with a
// "discovering…" suffix while discovery is still running. Each is a single
// line, so the layout doesn't shift between them.
func (m Model) footerView() string {
	footer := m.helpView()

	switch {
	case m.searching:
		footer = m.searchPrompt()
	case m.lastQuery != "":
		footer = m.searchStatus()
	}

	if !m.disc.done {
		footer += "  •  discovering…"
	}

	return footer
}

// searchPrompt renders the live search input, followed by a match count once
// the query is non-empty.
func (m Model) searchPrompt() string {
	view := m.searchInput.View()

	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return view
	}

	return view + "  •  " + matchSummary(m.matchCount(query))
}

// searchStatus renders the committed-search summary: the query, its match count,
// and the keys that cycle or clear it.
func (m Model) searchStatus() string {
	parts := []string{
		"/" + m.lastQuery,
		matchSummary(m.matchCount(m.lastQuery)),
		"n/N cycle",
		"esc clear",
	}

	return strings.Join(parts, "  •  ")
}

// matchSummary describes a match count in words.
func matchSummary(n int) string {
	switch n {
	case 0:
		return "no matches"
	case 1:
		return "1 match"
	default:
		return strconv.Itoa(n) + " matches"
	}
}

// helpView renders the navigation hint line.
func (m Model) helpView() string {
	parts := make([]string, 0, len(m.keys.ShortHelp()))
	for _, b := range m.keys.ShortHelp() {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}

	return strings.Join(parts, "  •  ")
}
