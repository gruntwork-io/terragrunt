package tui

import (
	"errors"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DiscoveryResult carries the outcome of the background discovery pass. The
// model receives it as a message and annotates the tree with the components.
type DiscoveryResult struct {
	Err        error
	Components component.Components
}

// ColorMode selects whether the browser colorizes its output. It mirrors the
// command's color decision so the TUI matches the rest of Terragrunt's output.
type ColorMode bool

const (
	// ColorDisabled renders the browser without ANSI color.
	ColorDisabled ColorMode = false
	// ColorEnabled renders the browser with ANSI color.
	ColorEnabled ColorMode = true
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
	dirCounts    map[string]dirCount
	resultCh     <-chan DiscoveryResult
	warnCh       <-chan viewtui.Warning
	home         string
	lastQuery    string
	toasts       viewtui.ToastStack
	keys         keyMap
	searchInput  textinput.Model
	searchOrigin int
	width        int
	height       int
	searching    bool
	gPending     bool
	color        ColorMode
	hasDarkBG    bool
	ready        bool
	done         bool
}

// ErrChannelsRequired is the panic value NewModel raises when its result or
// warning channel is nil. The browse command always supplies both, and the
// background listeners deadlock on a nil channel, so a nil here points at a
// caller (typically a test) that skipped the wiring rather than a runtime
// condition.
var ErrChannelsRequired = errors.New("browse: result and warning channels must not be nil")

// NewModel builds a Model rooted at the given tree. fs backs the on-demand reads
// of surrounding entries and file previews. resultCh delivers the background
// discovery result, and warnCh the warnings logged while it runs; both are
// required, and NewModel panics if either is nil.
func NewModel(
	l log.Logger,
	fs vfs.FS,
	root *Node,
	color ColorMode,
	resultCh <-chan DiscoveryResult,
	warnCh <-chan viewtui.Warning,
) Model {
	if resultCh == nil || warnCh == nil {
		panic(ErrChannelsRequired)
	}

	search := textinput.New()
	search.Prompt = "/"

	// Resolve the home directory once; the path bar abbreviates against it on
	// every render. An error leaves it empty, which disables abbreviation.
	home, err := os.UserHomeDir()
	if err != nil {
		l.Debugf("Could not resolve home directory for path abbreviation: %v", err)
	}

	return Model{
		root:        root,
		current:     root,
		cursor:      map[*Node]int{},
		colorizer:   dag.NewColorizer(bool(color)),
		fs:          fs,
		keys:        newKeyMap(),
		searchInput: search,
		index:       map[string]component.Component{},
		readFiles:   map[string]struct{}{},
		dirCounts:   map[string]dirCount{},
		resultCh:    resultCh,
		warnCh:      warnCh,
		home:        home,
		color:       color,
		hasDarkBG:   true,
	}
}

// Init implements tea.Model. It asks the terminal for its background color so
// the preview's syntax-highlight theme can match it, and starts listening for
// the background discovery result and the warnings logged while it runs.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.listenForResult(),
		viewtui.ListenForWarnings(m.warnCh),
	)
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
	m.computeCounts()
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

// nameMatches reports whether name contains query case-insensitively.
func nameMatches(name, query string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(query))
}

// matchCount returns how many entries in the current directory match query.
func (m Model) matchCount(query string) int {
	if query == "" {
		return 0
	}

	count := 0

	for _, n := range m.current.children {
		if nameMatches(n.name, query) {
			count++
		}
	}

	return count
}

// searchDirection is the direction nextMatch scans for the next matching entry.
type searchDirection int

const (
	searchForward  searchDirection = 1
	searchBackward searchDirection = -1
)

// nextMatch moves the cursor to the next entry matching the last committed
// query, scanning in direction dir and wrapping around the current directory.
// It's a no-op when no search has been committed.
func (m *Model) nextMatch(dir searchDirection) {
	if m.lastQuery == "" {
		return
	}

	children := m.current.children

	count := len(children)
	if count == 0 {
		return
	}

	cur := m.cursor[m.current]

	for i := 1; i <= count; i++ {
		idx := ((cur+int(dir)*i)%count + count) % count
		if nameMatches(children[idx].name, m.lastQuery) {
			m.cursor[m.current] = idx

			return
		}
	}
}

// firstMatch returns the index of the first child whose name contains query,
// case-insensitively, or -1 when none match.
func (m Model) firstMatch(query string) int {
	return slices.IndexFunc(m.current.children, func(n *Node) bool {
		return nameMatches(n.name, query)
	})
}
