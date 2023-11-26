package catalog

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

func Run(ctx *cli.Context, opts *options.TerragruntOptions) error {
	var moduleURL string

	if val := ctx.Args().Get(0); val != "" {
		moduleURL = val
	}

	_ = moduleURL

	return tui.Run(ctx.Context)
}

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
)

type item struct {
	title       string
	description string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.title }

type ListKeyMap struct {
	insertItem key.Binding
}

func newListKeyMap() *ListKeyMap {
	return &ListKeyMap{
		insertItem: key.NewBinding(
			key.WithKeys("+"),
			key.WithHelp("+", "add item"),
		),
	}
}

type model struct {
	list          list.Model
	itemGenerator *randomItemGenerator
	keys          *ListKeyMap
	delegateKeys  *delegateKeyMap
}

func newModel() model {
	var (
		itemGenerator randomItemGenerator
		delegateKeys  = newDelegateKeyMap()
		listKeys      = newListKeyMap()
	)

	// Make initial list of items
	const numItems = 64
	items := make([]list.Item, numItems)
	for i := 0; i < numItems; i++ {
		items[i] = itemGenerator.next()
	}

	// Setup list
	delegate := newItemDelegate(delegateKeys)
	groceryList := list.New(items, delegate, 0, 0)
	groceryList.Title = "Groceries"
	groceryList.Styles.Title = titleStyle
	groceryList.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			listKeys.insertItem,
		}
	}

	groceryList.KeyMap.CursorUp = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑/ctrl+p", "move up"),
	)

	groceryList.KeyMap.CursorDown = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓/ctrl+n", "move down"),
	)

	groceryList.KeyMap.GoToStart = key.NewBinding(
		key.WithKeys("home", "ctrl+a"),
		key.WithHelp("home/ctrl+a", "go to start"),
	)

	groceryList.KeyMap.GoToEnd = key.NewBinding(
		key.WithKeys("end", "ctrl+e"),
		key.WithHelp("end/ctrl+e", "go to end"),
	)

	groceryList.KeyMap.NextPage = key.NewBinding(
		key.WithKeys("right", "pgdown", "ctrl+v"),
		key.WithHelp("→/pgdn/ctrl+v", "next page"),
	)

	groceryList.KeyMap.PrevPage = key.NewBinding(
		key.WithKeys("left", "pgup", "alt+v"),
		key.WithHelp("←/pgup/alt+v", "prev page"),
	)

	return model{
		list:          groceryList,
		keys:          listKeys,
		delegateKeys:  delegateKeys,
		itemGenerator: &itemGenerator,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		// case key.Matches(msg, m.keys.toggleSpinner):
		// 	cmd := m.list.ToggleSpinner()
		// 	return m, cmd

		// case key.Matches(msg, m.keys.toggleTitleBar):
		// 	v := !m.list.ShowTitle()
		// 	m.list.SetShowTitle(v)
		// 	m.list.SetShowFilter(v)
		// 	m.list.SetFilteringEnabled(v)
		// 	return m, nil

		// case key.Matches(msg, m.keys.toggleStatusBar):
		// 	m.list.SetShowStatusBar(!m.list.ShowStatusBar())
		// 	return m, nil

		// case key.Matches(msg, m.keys.toggleHelpMenu):
		// 	m.list.SetShowHelp(!m.list.ShowHelp())
		// 	return m, nil

		case key.Matches(msg, m.keys.insertItem):
			return r, nil
			// m.delegateKeys.remove.SetEnabled(true)
			// newItem := m.itemGenerator.next()
			// insCmd := m.list.InsertItem(0, newItem)
			// statusCmd := m.list.NewStatusMessage(statusMessageStyle("Added " + newItem.Title()))
			// return m, tea.Batch(insCmd, statusCmd)
		}
	}

	// This will also call our delegate's update function.
	newListModel, cmd := m.list.Update(msg)
	m.list = newListModel
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	return appStyle.Render(m.list.View())
}

func run() {
	rand.Seed(time.Now().UTC().UnixNano())

	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
