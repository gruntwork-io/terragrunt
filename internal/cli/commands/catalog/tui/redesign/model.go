// Package redesign implements the redesigned catalog TUI experience with
// streaming component discovery and a welcome loading screen.
package redesign

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// sessionState keeps track of the view we are currently on.
type sessionState int

// button is a button in the buttonbar component.
type button int

const (
	titleForegroundColor = "#A8ACB1"
	titleBackgroundColor = "#1D252F"
)

const (
	ListState sessionState = iota
	PagerState
	ScaffoldState
)

const (
	scaffoldBtn button = iota
	viewSourceBtn
)

var (
	availableButtons = []button{scaffoldBtn, viewSourceBtn}
)

func (b button) String() string {
	return []string{
		"Scaffold",
		"View Source in Browser",
	}[b]
}

type Model struct {
	lists               [numTabs]list.Model
	logger              log.Logger
	terragruntOptions   *options.TerragruntOptions
	selectedComponent   *Component
	delegateKeys        *tui.DelegateKeyMap
	buttonBar           *buttonbar.ButtonBar
	componentCh         chan *ComponentEntry
	pagerKeys           tui.PagerKeyMap
	listKeys            list.KeyMap
	currentPagerButtons []button
	viewport            viewport.Model
	activeButton        button
	State               sessionState
	activeTab           tabKind
	height              int
	width               int
	ready               bool
	loading             bool
	userNavigated       bool
}

// List returns the currently active list — the one filtered by the active
// tab. Exposed for tests and view code that need to inspect items.
func (m Model) List() list.Model { //nolint:gocritic
	return m.lists[m.activeTab]
}

// ActiveTab returns which of the All/Modules/Templates tabs is focused.
func (m Model) ActiveTab() tabKind { //nolint:gocritic
	return m.activeTab
}

// NewModelStreaming creates a Model with a single initial entry and a channel
// for receiving additional entries as they are discovered.
func NewModelStreaming(l log.Logger, opts *options.TerragruntOptions, initial *ComponentEntry, componentCh chan *ComponentEntry) Model {
	items := []list.Item{initial}

	m := newModelWithItems(l, opts, items, componentCh)
	m.loading = true

	return m
}

func newModelWithItems(l log.Logger, opts *options.TerragruntOptions, items []list.Item, componentCh chan *ComponentEntry) Model {
	listKeys := tui.NewListKeyMap()
	delegateKeys := tui.NewDelegateKeyMap()
	pagerKeys := tui.NewPagerKeyMap()

	delegate := newCatalogDelegate(delegateKeys)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleForegroundColor)).
		Background(lipgloss.Color(titleBackgroundColor)).
		Padding(0, 1)

	var lists [numTabs]list.Model

	for i := range int(numTabs) {
		t := tabKind(i)

		tabItems := filterItemsByTab(items, t)

		lst := list.New(tabItems, delegate, 0, 0)
		lst.KeyMap = listKeys
		lst.SetFilteringEnabled(true)
		// The visible tab strip is rendered in the view; a per-list Title
		// is redundant, so clear it to keep the tab bar the only label.
		lst.Title = ""
		lst.SetShowTitle(false)
		lst.Styles.Title = titleStyle
		lists[i] = lst
	}

	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))

	bs := make([]string, len(availableButtons))
	for i, b := range availableButtons {
		bs[i] = b.String()
	}

	bb := buttonbar.New(bs)

	return Model{
		lists:             lists,
		listKeys:          listKeys,
		delegateKeys:      delegateKeys,
		viewport:          vp,
		buttonBar:         bb,
		pagerKeys:         pagerKeys,
		terragruntOptions: opts,
		logger:            l,
		componentCh:       componentCh,
	}
}

// filterItemsByTab returns the subset of items whose Kind belongs in tab t.
// TabAll returns everything unchanged.
func filterItemsByTab(items []list.Item, t tabKind) []list.Item {
	if t == TabAll {
		return items
	}

	out := make([]list.Item, 0, len(items))

	for _, it := range items {
		entry, ok := it.(*ComponentEntry)
		if !ok {
			continue
		}

		if t.matches(entry.Kind()) {
			out = append(out, entry)
		}
	}

	return out
}

// insertComponentSorted inserts a component into every tab whose filter
// accepts its kind (always TabAll, plus either TabModules or TabTemplates).
// Duplicates are skipped per-list by source path, and each list preserves
// its own cursor.
func (m *Model) insertComponentSorted(entry *ComponentEntry) tea.Cmd {
	if entry == nil {
		return nil
	}

	var cmds []tea.Cmd

	for i := range int(numTabs) {
		t := tabKind(i)
		if !t.matches(entry.Kind()) {
			continue
		}

		if cmd := m.insertIntoList(i, entry); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
}

// insertIntoList places entry into lists[idx] at the correct sorted
// position, skipping duplicates and preserving the per-list cursor.
func (m *Model) insertIntoList(idx int, entry *ComponentEntry) tea.Cmd {
	items := m.lists[idx].Items()
	entryTitle := entry.Title()

	// Binary search finds the insertion point by title for sort order.
	insertIdx := sort.Search(len(items), func(i int) bool {
		if existing, ok := items[i].(*ComponentEntry); ok {
			return strings.ToLower(existing.Title()) >= strings.ToLower(entryTitle)
		}

		return false
	})

	// De-duplicate by source path, not title, so distinct components that
	// share a display name are not collapsed.
	if isDuplicate(items, entry.Component.TerraformSourcePath()) {
		return nil
	}

	currentIdx := m.lists[idx].Index()

	cmd := m.lists[idx].InsertItem(insertIdx, entry)

	if m.userNavigated {
		// Preserve cursor: if we inserted before or at the current
		// selection, shift the cursor forward so it stays on the same item.
		if insertIdx <= currentIdx {
			m.lists[idx].Select(currentIdx + 1)
		}
	} else {
		// User hasn't navigated yet — keep cursor at the top.
		m.lists[idx].Select(0)
	}

	return cmd
}

// isDuplicate reports whether any item in the list has the same source path
// as sourcePath. This uses the stable TerraformSourcePath identity rather
// than the display title, so distinct components that share a title are not
// incorrectly collapsed.
func isDuplicate(items []list.Item, sourcePath string) bool {
	for _, item := range items {
		if existing, ok := item.(*ComponentEntry); ok {
			if existing.Component.TerraformSourcePath() == sourcePath {
				return true
			}
		}
	}

	return false
}

func (m Model) listenForComponent() tea.Cmd { //nolint:gocritic
	ch := m.componentCh
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}

		return componentMsg{entry: c}
	}
}

// Init implements bubbletea.Model.Init
func (m Model) Init() tea.Cmd { //nolint:gocritic
	cmds := []tea.Cmd{m.buttonBar.Init()}

	if m.componentCh != nil {
		cmds = append(cmds, m.listenForComponent())
	}

	return tea.Batch(cmds...)
}
