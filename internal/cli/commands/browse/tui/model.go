package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
)

// Model is the bubbletea model backing the Miller-columns browser.
type Model struct {
	root      *Node
	current   *Node
	cursor    map[*Node]int
	colorizer *dag.Colorizer
	// fs reads directories and files on demand: the surrounding entries shown
	// for context and the highlighted file's preview. bubbletea's Update and
	// View have fixed signatures, so the handler lives on the model rather than
	// being threaded through.
	fs     vfs.FS
	keys   keyMap
	width  int
	height int
	// shouldColor mirrors the list command's color decision; it gates syntax
	// highlighting in the file preview.
	shouldColor bool
	// hasDarkBG tracks the terminal background so the preview's highlight theme
	// matches it. It defaults to dark and is corrected on the first background
	// color report.
	hasDarkBG bool
	ready     bool
}

// NewModel builds a Model rooted at the given tree. shouldColor mirrors the
// list command's color decision so the TUI matches the rest of the output. fs
// backs the on-demand reads of surrounding entries and file previews.
func NewModel(fs vfs.FS, root *Node, shouldColor bool) Model {
	return Model{
		root:        root,
		current:     root,
		cursor:      map[*Node]int{},
		colorizer:   dag.NewColorizer(shouldColor),
		fs:          fs,
		keys:        newKeyMap(),
		shouldColor: shouldColor,
		hasDarkBG:   true,
	}
}

// Init implements tea.Model. It asks the terminal for its background color so
// the preview's syntax-highlight theme can match it.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Current returns the directory whose contents fill the focused column.
func (m Model) Current() *Node { return m.current }

// Selected returns the highlighted child of the current directory, or nil when
// the current directory is empty.
func (m Model) Selected() *Node {
	children := m.current.children

	idx := m.cursor[m.current]
	if idx < 0 || idx >= len(children) {
		return nil
	}

	return children[idx]
}

// moveCursor shifts the cursor within the current directory, clamped to range.
func (m *Model) moveCursor(delta int) {
	count := len(m.current.children)
	if count == 0 {
		m.cursor[m.current] = 0

		return
	}

	idx := m.cursor[m.current] + delta
	idx = max(idx, 0)
	idx = min(idx, count-1)

	m.cursor[m.current] = idx
}
