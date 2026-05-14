// Package redesign implements the redesigned catalog TUI experience with
// streaming component discovery and a welcome loading screen.
package redesign

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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

type Model struct {
	lists               [numTabs]list.Model
	logger              log.Logger
	terragruntOptions   *options.TerragruntOptions
	selectedComponent   *Component
	delegateKeys        *tui.DelegateKeyMap
	buttonBar           *buttonbar.ButtonBar
	componentCh         chan *ComponentEntry
	errCh               chan error
	mdRenderer          *glamour.TermRenderer
	pagerKeys           tui.PagerKeyMap
	listKeys            list.KeyMap
	currentPagerButtons []button
	exitMessage         string
	viewport            viewport.Model
	venv                venv.Venv
	activeButton        button
	State               sessionState
	activeTab           tabKind
	height              int
	width               int
	mdRendererWidth     int
	ready               bool
	loading             bool
	userNavigated       bool
	hasDarkBG           bool
	mdRendererDark      bool
}

// NewModelStreaming creates a Model with a single initial entry and a channel
// for receiving additional entries as they are discovered. errCh carries the
// loadFunc result; the streaming Model drains it after componentCh closes so
// it can synthesize a DiscoveryCompleteMsg without racing the welcome model.
func NewModelStreaming(
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	initial *ComponentEntry,
	componentCh chan *ComponentEntry,
	errCh chan error,
) Model {
	items := []list.Item{initial}

	m := newModelWithItems(l, v, opts, items, componentCh)
	m.errCh = errCh
	m.loading = true

	return m
}

// NewModelWithExitMessageForTest returns a Model whose only populated field
// is the exit message, for tests that exercise post-exit message emission.
func NewModelWithExitMessageForTest(msg string) Model {
	return Model{exitMessage: msg}
}

// ActiveTab returns which of the All/Modules/Templates tabs is focused.
func (m Model) ActiveTab() tabKind {
	return m.activeTab
}

// ExitMessage returns the styled post-exit message the model set while
// handling its final action (e.g., a successful copy that generated a
// terragrunt.values.hcl file). The caller is responsible for printing it
// after the tea.Program returns, once the alt screen has been torn down.
func (m Model) ExitMessage() string {
	return m.exitMessage
}

// Init implements bubbletea.Model.Init
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.buttonBar.Init(),
		// Reply arrives as tea.BackgroundColorMsg; cached so the README
		// renderer doesn't have to issue an OSC 11 round-trip per click.
		tea.RequestBackgroundColor,
	}

	if m.componentCh != nil {
		cmds = append(cmds, m.listenForComponent())
	}

	return tea.Batch(cmds...)
}

// List returns the currently active list, the one filtered by the active
// tab. Exposed for tests and view code that need to inspect items.
func (m Model) List() list.Model {
	return m.lists[m.activeTab]
}

func (b button) String() string {
	return []string{
		"Scaffold",
		"View Source in Browser",
	}[b]
}

// insertComponentSorted inserts a component into every tab whose filter
// accepts it (always TabAll, plus the tab matching its native Kind, plus any
// tab whose name appears in the component's front-matter tags). Duplicates
// are skipped per-list by source path, and each list preserves its own
// cursor.
func (m *Model) insertComponentSorted(entry *ComponentEntry) tea.Cmd {
	if entry == nil {
		return nil
	}

	var cmds []tea.Cmd

	for i := range int(numTabs) {
		t := tabKind(i)
		if !t.matches(entry) {
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
		// User hasn't navigated yet, so keep the cursor at the top.
		m.lists[idx].Select(0)
	}

	return cmd
}

// listenForComponent mirrors the welcome model's variant: the next component
// flows through as componentMsg, and a closed componentCh produces a
// DiscoveryCompleteMsg with the loadFunc error drained from errCh. See
// WelcomeModel.listenForComponent for why completion shares this Cmd
// rather than living in a sibling.
func (m Model) listenForComponent() tea.Cmd {
	ch := m.componentCh
	if ch == nil {
		return nil
	}

	errCh := m.errCh

	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			var err error
			if errCh != nil {
				err = <-errCh
			}

			return DiscoveryCompleteMsg{Err: err}
		}

		return componentMsg{entry: c}
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

		if t.matches(entry) {
			out = append(out, entry)
		}
	}

	return out
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

func newModelWithItems(
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	items []list.Item,
	componentCh chan *ComponentEntry,
) Model {
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
		venv:              v,
		// Matches lipgloss.HasDarkBackground's fallback. Corrected on the
		// first tea.BackgroundColorMsg.
		hasDarkBG: true,
	}
}
