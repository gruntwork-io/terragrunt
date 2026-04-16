package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	delegateKeys        *DelegateKeyMap
	buttonBar           *buttonbar.ButtonBar
	pagerKeys           PagerKeyMap
	listKeys            list.KeyMap
	currentPagerButtons []button
	viewport            viewport.Model
	activeButton        button
	State               sessionState
	height              int
	width               int
	ready               bool
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

	listKeys := NewListKeyMap()
	delegateKeys := NewDelegateKeyMap()
	pagerKeys := NewPagerKeyMap()

	delegate := NewItemDelegate(delegateKeys)
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
	}
}

// Init implements bubbletea.Model.Init
func (m Model) Init() tea.Cmd { //nolint:gocritic
	return m.buttonBar.Init()
}
