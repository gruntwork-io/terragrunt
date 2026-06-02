package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
)

// Model is the bubbletea model backing the Miller-columns browser.
type Model struct {
	fs           vfs.FS
	current      *Node
	cursor       map[*Node]int
	colorizer    *dag.Colorizer
	root         *Node
	lastQuery    string
	keys         keyMap
	searchInput  textinput.Model
	searchOrigin int
	width        int
	height       int
	searching    bool
	shouldColor  bool
	hasDarkBG    bool
	ready        bool
}

// NewModel builds a Model rooted at the given tree. shouldColor mirrors the
// list command's color decision so the TUI matches the rest of the output. fs
// backs the on-demand reads of surrounding entries and file previews.
func NewModel(fs vfs.FS, root *Node, shouldColor bool) Model {
	search := textinput.New()
	search.Prompt = "/"

	return Model{
		root:        root,
		current:     root,
		cursor:      map[*Node]int{},
		colorizer:   dag.NewColorizer(shouldColor),
		fs:          fs,
		keys:        newKeyMap(),
		searchInput: search,
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

// startSearch opens the search input over the current directory, remembering
// the cursor so a cancel can restore it. The returned command drives the
// textinput's cursor blink and must reach the Bubble Tea loop.
func (m *Model) startSearch() tea.Cmd {
	m.searching = true
	m.searchOrigin = m.cursor[m.current]
	m.searchInput.SetValue("")

	return m.searchInput.Focus()
}

// applySearch moves the cursor to the first entry matching the typed query as
// the user types. An empty query returns the cursor to where the search began;
// a query with no match leaves the cursor where it is.
func (m *Model) applySearch() {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.cursor[m.current] = m.searchOrigin

		return
	}

	if idx := m.firstMatch(query); idx >= 0 {
		m.cursor[m.current] = idx
	}
}

// commitSearch closes the search input, keeping the cursor on the current match
// and remembering the query so NextMatch and PrevMatch can cycle from here.
func (m *Model) commitSearch() {
	m.searchInput.Blur()
	m.searching = false
	m.lastQuery = strings.TrimSpace(m.searchInput.Value())
}

// cancelSearch closes the search input, clears the active search, and restores
// the cursor to where the search began.
func (m *Model) cancelSearch() {
	m.searchInput.Blur()
	m.searchInput.SetValue("")
	m.searching = false
	m.lastQuery = ""
	m.cursor[m.current] = m.searchOrigin
}

// activeQuery returns the query whose matches should be marked: the live input
// while typing, the committed query afterward, or "" when no search is active.
func (m Model) activeQuery() string {
	if m.searching {
		return strings.TrimSpace(m.searchInput.Value())
	}

	return m.lastQuery
}

// matchCount returns how many entries in the current directory match query.
func (m Model) matchCount(query string) int {
	if query == "" {
		return 0
	}

	q := strings.ToLower(query)
	count := 0

	for _, n := range m.current.children {
		if strings.Contains(strings.ToLower(n.name), q) {
			count++
		}
	}

	return count
}

// nextMatch moves the cursor to the next entry matching the last committed
// query, scanning in direction dir (+1 forward, -1 backward) and wrapping around
// the current directory. It's a no-op when no search has been committed.
func (m *Model) nextMatch(dir int) {
	if m.lastQuery == "" {
		return
	}

	children := m.current.children

	count := len(children)
	if count == 0 {
		return
	}

	query := strings.ToLower(m.lastQuery)
	cur := m.cursor[m.current]

	for i := 1; i <= count; i++ {
		idx := ((cur+dir*i)%count + count) % count
		if strings.Contains(strings.ToLower(children[idx].name), query) {
			m.cursor[m.current] = idx

			return
		}
	}
}

// firstMatch returns the index of the first child whose name contains query,
// case-insensitively, or -1 when none match.
func (m Model) firstMatch(query string) int {
	q := strings.ToLower(query)

	for i, n := range m.current.children {
		if strings.Contains(strings.ToLower(n.name), q) {
			return i
		}
	}

	return -1
}
