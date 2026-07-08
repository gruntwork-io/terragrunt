// Package tui implements the catalog TUI experience with streaming component
// discovery and a welcome loading screen.
package tui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// EmitExitMessage prints any post-exit message that the final model stashed
// during its session (e.g. the values-stub callout on a successful copy, or
// the discovery-failure details from the welcome screen). It runs after the
// tea program restores the main terminal buffer, because messages queued via
// tea.Printf while the alt screen is active get discarded when the alt
// screen is torn down.
func EmitExitMessage(finalModel tea.Model, errWriter io.Writer, l log.Logger) {
	type exitMessenger interface{ ExitMessage() string }

	m, ok := finalModel.(exitMessenger)
	if !ok || m.ExitMessage() == "" {
		return
	}

	if _, err := fmt.Fprintln(errWriter, m.ExitMessage()); err != nil {
		l.Warnf("Failed to write exit message: %v", err)
	}
}

type sessionState int

type button int

const (
	titleForegroundColor = "#A8ACB1"
	titleBackgroundColor = "#1D252F"
)

const (
	ListState sessionState = iota
	PagerState
	FormState
	ScaffoldState
)

const (
	scaffoldBtn button = iota
	viewSourceBtn
)

var availableButtons = []button{scaffoldBtn, viewSourceBtn}

type Model struct {
	// ctx is the welcome layer's cancellable context. Long-running off-UI
	// work (e.g. scaffold.Prepare downloading sources) propagates the
	// user's Ctrl+C through this context, so the call returns instead of
	// blocking on an abandoned download.
	ctx context.Context
	// lists holds one list per tab; activeTab indexes it. A component is
	// inserted into every list whose tab filter accepts it, so the same
	// entry can appear under All and its kind-specific tab at once.
	lists [numTabs]list.Model
	// logger surfaces non-fatal diagnostics (e.g. an unknown button press)
	// and is handed down to the scaffold/copy/form-discovery leaves.
	logger log.Logger
	// terragruntOptions carries the resolved CLI options that drive
	// discovery and scaffolding; it is threaded into every leaf operation.
	terragruntOptions *options.TerragruntOptions
	// selectedComponent is the entry the user acted on. It is carried into
	// the pager and form so scaffold and "view source" know their target
	// even after the list selection moves on.
	selectedComponent *Component
	// delegateKeys are the list-row keybindings (choose, interactive
	// scaffold) matched while in the list view.
	delegateKeys *DelegateKeyMap
	// buttonBar is the pager's action strip (Scaffold / View Source). It is
	// rebuilt per component because the available actions depend on kind.
	buttonBar *buttonbar.ButtonBar
	// componentCh streams discovered components from the background loader.
	// A nil channel disables the listener (used by the non-streaming test
	// constructor).
	componentCh chan *ComponentEntry
	// errCh carries the loader's final result, drained after componentCh
	// closes so completion is observed only after every component.
	errCh chan error
	// warnCh carries warnings captured from the background loaders. The
	// welcome model arms the session's single listener and hands the channel
	// over at the model swap; the Warning handler re-arms it here.
	warnCh <-chan viewtui.Warning
	// toasts is the stack of floating warning notifications composited over
	// the view, carried over from the welcome model at the swap.
	toasts viewtui.ToastStack
	// mdRenderer is the cached glamour renderer for README markdown. It is
	// reused while mdRendererWidth and mdRendererDark still match the
	// current width and background, and rebuilt otherwise.
	mdRenderer *glamour.TermRenderer
	// form is the variable-entry form, nil until a formReadyMsg arrives
	// from scaffold-variable discovery.
	form *FormModel
	// scaffoldPlan is the prepared plan backing the active form. Its
	// temporary source download is cleaned up when the form is abandoned
	// or the scaffold is consumed.
	scaffoldPlan *scaffold.Plan
	// valuesRefs holds the `values.*` references collected from a copyable
	// unit/stack, used to build that component's values form.
	valuesRefs *ValuesReferences
	// terminalErr is the failure that ended the session (a scaffold, copy,
	// or form-discovery error). Run returns it so the catalog command exits
	// nonzero; a deliberate quit leaves it nil.
	terminalErr error
	// loadErr is a non-fatal discovery failure (some catalog sources failed
	// to load while others produced components). It renders as a notice in
	// the list view rather than ending the session.
	loadErr error
	// venv is the root virtualized environment threaded from the CLI
	// entrypoint, so scaffold and form-discovery leaves run against the
	// same filesystem and exec handles as the rest of Terragrunt.
	venv venv.Venv
	// pagerKeys are the keybindings handled while reading a README in the
	// pager view.
	pagerKeys PagerKeyMap
	// listKeys are the list-view keybindings, including quit.
	listKeys list.KeyMap
	// currentPagerButtons are the actions offered for the component in the
	// pager; activeButton indexes into it.
	currentPagerButtons []button
	// exitMessage is the styled message stashed for printing after the alt
	// screen tears down (a success callout or a failure notice), since
	// writes during the alt screen are discarded.
	exitMessage string
	// viewport is the scrollable pager that renders README content.
	viewport viewport.Model
	// activeButton is the focused entry in currentPagerButtons.
	activeButton button
	// State is the current view (list, pager, form, or scaffold).
	State sessionState
	// priorState is the view to return to when a form is cancelled,
	// recorded when the form is entered.
	priorState sessionState
	// activeTab is the focused tab (All / Modules / Templates / Stacks);
	// it indexes lists.
	activeTab TabKind
	// height is the last terminal height, refreshed on each WindowSizeMsg
	// and used to size the viewport and form.
	height int
	// width is the last terminal width, refreshed on each WindowSizeMsg and
	// used to size the viewport, form, and markdown renderer.
	width int
	// mdRendererWidth is the width the cached mdRenderer was built for; a
	// change invalidates it.
	mdRendererWidth int
	// ready reports whether the first WindowSizeMsg has sized the viewport,
	// gating its one-time creation.
	ready bool
	// loading reports whether discovery is still streaming components; it
	// drives the "(loading...)" suffix on the tab bar.
	loading bool
	// userNavigated reports whether the user has moved the list cursor.
	// Until they do, newly streamed components keep the selection pinned to
	// the top instead of shifting it.
	userNavigated bool
	// hasDarkBG is the detected terminal background brightness. It selects
	// the markdown style and, when it changes, invalidates the cached
	// renderer.
	hasDarkBG bool
	// mdRendererDark is the background brightness the cached mdRenderer was
	// built for; a change invalidates it.
	mdRendererDark bool
	// softWrap toggles glamour's word-wrap in the pager view. Default true
	// matches the prior behavior; the `w` key flips it so users reading a
	// README with intentionally long lines (ascii diagrams, wide tables)
	// can see them as-authored.
	softWrap bool
}

// NewModelStreaming creates a Model with a single initial entry and a channel
// for receiving additional entries as they are discovered. errCh carries the
// loadFunc result; the streaming Model drains it after componentCh closes so
// it can synthesize a DiscoveryCompleteMsg without racing the welcome model.
// ctx is the cancellable context the welcome layer hands down so off-UI work
// can observe Ctrl+C.
func NewModelStreaming(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	initial *ComponentEntry,
	componentCh chan *ComponentEntry,
	errCh chan error,
) Model {
	items := []list.Item{initial}

	m := newModelWithItems(l, v, opts, items, componentCh)
	m.ctx = ctx
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
func (m Model) ActiveTab() TabKind {
	return m.activeTab
}

// Loading reports whether discovery is still running. When true, the tab
// strip renders a "(loading...)" suffix.
func (m Model) Loading() bool {
	return m.loading
}

// Err returns the failure that ended the session, or nil when the session
// ended without one (a deliberate quit). Run propagates it so the catalog
// command exits nonzero after an in-TUI failure.
func (m Model) Err() error {
	return m.terminalErr
}

// SoftWrap reports whether the pager's glamour renderer wraps long
// lines at terminal width. Exposed for tests that drive the `w` key
// and verify the toggle.
func (m Model) SoftWrap() bool {
	return m.softWrap
}

// ExitMessage returns the styled post-exit message the model set while
// handling its final action (e.g., a successful copy that generated a
// terragrunt.values.hcl file). The caller is responsible for printing it
// after the tea.Program returns, once the alt screen has been torn down.
func (m Model) ExitMessage() string {
	return m.exitMessage
}

// Init implements bubbletea.Model.Init.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.buttonBar.Init(),
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
func (m Model) insertComponentSorted(entry *ComponentEntry) (Model, tea.Cmd) {
	if entry == nil {
		return m, nil
	}

	var cmds []tea.Cmd

	for i := range int(numTabs) {
		t := TabKind(i)
		if !t.Matches(entry) {
			continue
		}

		var cmd tea.Cmd

		m, cmd = m.insertIntoList(i, entry)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) == 0 {
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// insertIntoList places entry into lists[idx] at the correct sorted
// position, skipping duplicates and preserving the per-list cursor.
func (m Model) insertIntoList(idx int, entry *ComponentEntry) (Model, tea.Cmd) {
	items := m.lists[idx].Items()
	entryTitle := entry.Title()

	insertIdx := sort.Search(len(items), func(i int) bool {
		if existing, ok := items[i].(*ComponentEntry); ok {
			return strings.ToLower(existing.Title()) >= strings.ToLower(entryTitle)
		}

		return false
	})

	if isDuplicate(items, entry.Component.TerraformSourcePath()) {
		return m, nil
	}

	currentIdx := m.lists[idx].Index()

	cmd := m.lists[idx].InsertItem(insertIdx, entry)

	if !m.userNavigated {
		m.lists[idx].Select(0)

		return m, cmd
	}

	if insertIdx <= currentIdx {
		m.lists[idx].Select(currentIdx + 1)
	}

	return m, cmd
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
func filterItemsByTab(items []list.Item, t TabKind) []list.Item {
	if t == TabAll {
		return items
	}

	out := make([]list.Item, 0, len(items))

	for _, it := range items {
		entry, ok := it.(*ComponentEntry)
		if !ok {
			continue
		}

		if t.Matches(entry) {
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
	listKeys := NewListKeyMap()
	delegateKeys := NewDelegateKeyMap()
	pagerKeys := NewPagerKeyMap()

	delegate := newCatalogDelegate(delegateKeys)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleForegroundColor)).
		Background(lipgloss.Color(titleBackgroundColor)).
		Padding(0, 1)

	var lists [numTabs]list.Model

	for i := range int(numTabs) {
		t := TabKind(i)

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
		// Soft-wrap on by default keeps glamour wrapping at terminal width
		// like before; `w` flips it.
		softWrap: true,
	}
}
