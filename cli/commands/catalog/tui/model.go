package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	terragruntOptions   *options.TerragruntOptions
	SVC                 catalog.CatalogService
	selectedModule      *module.Module
	delegateKeys        *delegateKeyMap
	buttonBar           *buttonbar.ButtonBar
	currentPagerButtons []button
	pagerKeys           pagerKeyMap
	listKeys            list.KeyMap
	viewport            viewport.Model
	activeButton        button
	State               sessionState
	height              int
	width               int
	ready               bool
}

func NewModel(l log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService) Model {
	var (
		modules      = svc.Modules()
		items        = make([]list.Item, 0, len(modules))
		listKeys     = newListKeyMap()
		delegateKeys = newDelegateKeyMap()
		pagerKeys    = newPagerKeyMap()
	)

	// Make the initial list of items
	for _, module := range modules {
		items = append(items, module)
	}

	// Setup the list
	delegate := newItemDelegate(delegateKeys)
	list := list.New(items, delegate, 0, 0)
	list.KeyMap = listKeys
	list.SetFilteringEnabled(true)
	list.Title = title
	list.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleForegroundColor)).
		Background(lipgloss.Color(titleBackgroundColor)).
		Padding(0, 1)

	// Setup the markdown viewer
	vp := viewport.New(0, 0)

	// Setup the button bar
	bs := make([]string, len(availableButtons))
	for i, b := range availableButtons {
		bs[i] = b.String()
	}

	bb := buttonbar.New(bs)

	return Model{
		List:              list,
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
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.buttonBar.Init(),
	)
}
