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
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// sessionState keeps track of the view we are currently on.
type sessionState int

// button is a button in the buttonbar component.
type button int

const (
	title = "List of Modules"

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
	List                list.Model
	logger              log.Logger
	SVC                 catalog.CatalogService
	terragruntOptions   *options.TerragruntOptions
	selectedModule      *module.Module
	delegateKeys        *tui.DelegateKeyMap
	buttonBar           *buttonbar.ButtonBar
	moduleCh            chan *module.Module
	pagerKeys           tui.PagerKeyMap
	listKeys            list.KeyMap
	currentPagerButtons []button
	viewport            viewport.Model
	activeButton        button
	State               sessionState
	height              int
	width               int
	ready               bool
	loading             bool
	userNavigated       bool
}

func NewModel(l log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService) Model {
	modules := svc.Modules()
	items := make([]list.Item, 0, len(modules))

	for _, mod := range modules {
		items = append(items, mod)
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].(*module.Module).Title()) < strings.ToLower(items[j].(*module.Module).Title())
	})

	return newModelWithItems(l, opts, svc, items, nil)
}

// NewModelStreaming creates a Model with a single initial module and a channel
// for receiving additional modules as they are discovered.
func NewModelStreaming(l log.Logger, opts *options.TerragruntOptions, initial *module.Module, moduleCh chan *module.Module) Model {
	items := []list.Item{initial}

	m := newModelWithItems(l, opts, nil, items, moduleCh)
	m.loading = true

	return m
}

func newModelWithItems(l log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService, items []list.Item, moduleCh chan *module.Module) Model {
	listKeys := tui.NewListKeyMap()
	delegateKeys := tui.NewDelegateKeyMap()
	pagerKeys := tui.NewPagerKeyMap()

	delegate := tui.NewItemDelegate(delegateKeys)
	lst := list.New(items, delegate, 0, 0)
	lst.KeyMap = listKeys
	lst.SetFilteringEnabled(true)
	lst.Title = title
	lst.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleForegroundColor)).
		Background(lipgloss.Color(titleBackgroundColor)).
		Padding(0, 1)

	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))

	bs := make([]string, len(availableButtons))
	for i, b := range availableButtons {
		bs[i] = b.String()
	}

	bb := buttonbar.New(bs)

	return Model{
		List:              lst,
		listKeys:          listKeys,
		delegateKeys:      delegateKeys,
		viewport:          vp,
		buttonBar:         bb,
		pagerKeys:         pagerKeys,
		terragruntOptions: opts,
		SVC:               svc,
		logger:            l,
		moduleCh:          moduleCh,
	}
}

// insertModuleSorted inserts a module into the list in alphabetical order,
// skipping duplicates. If the user has started navigating, the cursor stays
// on the currently selected item. Otherwise it stays at the top of the list.
func (m *Model) insertModuleSorted(mod *module.Module) tea.Cmd {
	items := m.List.Items()
	modTitle := mod.Title()

	// Binary search finds the insertion point and doubles as a duplicate check:
	// if the item at insertIdx matches, the module is already in the list.
	insertIdx := sort.Search(len(items), func(i int) bool {
		if existing, ok := items[i].(*module.Module); ok {
			return strings.ToLower(existing.Title()) >= strings.ToLower(modTitle)
		}

		return false
	})

	if isDuplicate(items, insertIdx, modTitle) {
		return nil
	}

	currentIdx := m.List.Index()

	cmd := m.List.InsertItem(insertIdx, mod)

	if m.userNavigated {
		// Preserve cursor: if we inserted before or at the current
		// selection, shift the cursor forward so it stays on the same item.
		if insertIdx <= currentIdx {
			m.List.Select(currentIdx + 1)
		}
	} else {
		// User hasn't navigated yet — keep cursor at the top.
		m.List.Select(0)
	}

	return cmd
}

// isDuplicate reports whether the item at idx in the sorted list has the
// same title (case-insensitive) as modTitle.
func isDuplicate(items []list.Item, idx int, modTitle string) bool {
	if idx >= len(items) {
		return false
	}

	existing, ok := items[idx].(*module.Module)
	if !ok {
		return false
	}

	return strings.EqualFold(existing.Title(), modTitle)
}

func (m Model) listenForModule() tea.Cmd { //nolint:gocritic
	ch := m.moduleCh
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		mod, ok := <-ch
		if !ok {
			return nil
		}

		return moduleMsg{module: mod}
	}
}

// Init implements bubbletea.Model.Init
func (m Model) Init() tea.Cmd { //nolint:gocritic
	cmds := []tea.Cmd{m.buttonBar.Init()}

	if m.moduleCh != nil {
		cmds = append(cmds, m.listenForModule())
	}

	return tea.Batch(cmds...)
}
