package tui

import (
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
)

// DiscoveryResult carries the outcome of the background discovery pass. The
// model receives it as a message and annotates the tree with the components.
type DiscoveryResult struct {
	Err        error
	Components component.Components
}

// Warning is a warn-or-worse log entry captured while background discovery
// runs. The model receives it as a message and surfaces it as a toast.
type Warning struct {
	Message string
}

// ToastExpired dismisses the toast with the given ID. Each toast schedules
// its own expiry when pushed.
type ToastExpired struct {
	ID int
}

// toast is a single on-screen notification, identified so its scheduled
// expiry can dismiss it.
type toast struct {
	message string
	id      int
}

const (
	// toastTTL is how long a toast stays on screen before it expires.
	toastTTL = 5 * time.Second

	// maxToasts caps how many toasts are shown at once; pushing past the cap
	// drops the oldest.
	maxToasts = 3
)

// Model is the bubbletea model backing the Miller-columns browser.
type Model struct {
	fs           vfs.FS
	current      *Node
	cursor       map[*Node]int
	colorizer    *dag.Colorizer
	root         *Node
	index        map[string]component.Component
	readFiles    map[string]struct{}
	resultCh     <-chan DiscoveryResult
	warnCh       <-chan Warning
	lastQuery    string
	toasts       []toast
	keys         keyMap
	searchInput  textinput.Model
	lastToastID  int
	searchOrigin int
	width        int
	height       int
	searching    bool
	gPending     bool
	shouldColor  bool
	hasDarkBG    bool
	ready        bool
	done         bool
}

// NewModel builds a Model rooted at the given tree. shouldColor mirrors the
// command's color decision so the TUI matches the rest of Terragrunt's output.
// fs backs the on-demand reads of surrounding entries and file previews.
// resultCh delivers the background discovery result, and warnCh the warnings
// logged while it runs; either is nil when there is none.
func NewModel(fs vfs.FS, root *Node, shouldColor bool, resultCh <-chan DiscoveryResult, warnCh <-chan Warning) Model {
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
		index:       map[string]component.Component{},
		readFiles:   map[string]struct{}{},
		resultCh:    resultCh,
		warnCh:      warnCh,
		shouldColor: shouldColor,
		hasDarkBG:   true,
	}
}

// Init implements tea.Model. It asks the terminal for its background color so
// the preview's syntax-highlight theme can match it, and starts listening for
// the background discovery result and warnings when there are any.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor}

	if m.resultCh != nil {
		cmds = append(cmds, m.listenForResult())
	}

	if m.warnCh != nil {
		cmds = append(cmds, m.listenForWarnings())
	}

	return tea.Batch(cmds...)
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

// listenForResult blocks on the discovery channel and delivers the result as a
// message. Discovery produces exactly one result, so this Cmd is not re-armed.
func (m Model) listenForResult() tea.Cmd {
	ch := m.resultCh

	return func() tea.Msg {
		return <-ch
	}
}

// listenForWarnings blocks on the warnings channel and delivers the next
// warning as a message. Unlike the discovery result, warnings keep coming, so
// the Warning handler re-arms this Cmd after each one. A closed channel
// delivers nothing and stops the re-arming.
func (m Model) listenForWarnings() tea.Cmd {
	ch := m.warnCh

	return func() tea.Msg {
		w, ok := <-ch
		if !ok {
			return nil
		}

		return w
	}
}

// pushToast adds a toast with the given message, dropping the oldest once the
// stack is full. The returned command schedules the toast's expiry.
func (m *Model) pushToast(message string) tea.Cmd {
	m.lastToastID++
	m.toasts = append(m.toasts, toast{id: m.lastToastID, message: message})

	if len(m.toasts) > maxToasts {
		m.toasts = m.toasts[len(m.toasts)-maxToasts:]
	}

	id := m.lastToastID

	return tea.Tick(toastTTL, func(time.Time) tea.Msg {
		return ToastExpired{ID: id}
	})
}

// dropToast removes the toast with the given ID; expiry of an already-dropped
// toast is a no-op.
func (m *Model) dropToast(id int) {
	m.toasts = slices.DeleteFunc(m.toasts, func(t toast) bool {
		return t.id == id
	})
}

// applyDiscovery records the discovery result and annotates the loaded tree, so
// later renders resolve counts, metadata, and read-file highlighting in place of
// their loading placeholders. A failed discovery still applies whatever partial
// components it produced; the caller flags the failure as a toast.
func (m *Model) applyDiscovery(res DiscoveryResult) {
	m.index = make(map[string]component.Component, len(res.Components))
	m.readFiles = map[string]struct{}{}

	for _, c := range res.Components {
		m.index[c.Path()] = c

		for _, f := range c.Reading() {
			m.readFiles[f] = struct{}{}
		}
	}

	m.attachComponents(m.root)
	m.done = true
}

// attachComponents walks the loaded tree and attaches each node's discovered
// component, refining its kind to discovery's authority.
func (m *Model) attachComponents(n *Node) {
	if c, ok := m.index[n.absPath]; ok {
		n.component = c
		n.kind = kindForComponent(c)
	}

	for _, child := range n.children {
		m.attachComponents(child)
	}
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
