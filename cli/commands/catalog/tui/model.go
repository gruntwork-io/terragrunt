package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/options"
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
	listState sessionState = iota
	pagerState
	scaffoldState
)

const (
	scaffoldBtn button = iota
	viewSourceBtn
	lastBtn
)

func (b button) String() string {
	return []string{
		"Scaffold",
		"View Source in Browser",
	}[b]
}

type model struct {
	// globals
	state  sessionState
	width  int
	height int

	// list view
	list         list.Model
	listKeys     list.KeyMap
	delegateKeys *delegateKeyMap

	// pager view
	ready           bool
	viewport        viewport.Model
	buttonBar       *buttonbar.ButtonBar
	activeButton    button
	releaseNotesURL string
	pagerKeys       pagerKeyMap
	selectedModule  *module.Module

	terragruntOptions *options.TerragruntOptions
}

func newModel(modules module.Modules, opts *options.TerragruntOptions) model {
	var (
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
	bs := make([]string, lastBtn)
	for i, b := range []button{scaffoldBtn, viewSourceBtn} {
		bs[i] = b.String()
	}

	bb := buttonbar.New(bs)

	return model{
		list:              list,
		listKeys:          listKeys,
		delegateKeys:      delegateKeys,
		viewport:          vp,
		buttonBar:         bb,
		pagerKeys:         pagerKeys,
		terragruntOptions: opts,
	}
}

// Init implements bubbletea.Model.Init
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.buttonBar.Init(),
	)
}
