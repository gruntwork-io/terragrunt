package list

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/models/page"
)

const (
	title = "List of Modules"

	titleForegroundColor = "#A8ACB1"
	titleBackgroundColor = "#1D252F"
)

type Model struct {
	*list.Model
	delegate *Delegate
}

func NewModel(modules module.Items) *Model {
	var items []list.Item
	for _, module := range modules {
		items = append(items, module)
	}

	delegate := NewDelegate()

	model := list.New(items, delegate, 0, 0)
	model.KeyMap = NewKeyMap()
	model.SetFilteringEnabled(true)
	model.Title = title
	model.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleForegroundColor)).
		Background(lipgloss.Color(titleBackgroundColor)).
		Padding(0, 1)

	return &Model{
		Model:    &model,
		delegate: delegate,
	}
}

func (model Model) Init() tea.Cmd {
	return nil
}

func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := lipgloss.NewStyle().Padding(1, 2).GetFrameSize()
		model.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if model.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, model.delegate.Choose):
			if module, ok := model.SelectedItem().(*module.Item); ok {
				model, err := page.NewModel(module, model.Width(), model.Height(), model)
				if err != nil {
					fmt.Println("Could not initialize Bubble Tea model:", err)
					os.Exit(1)
				}

				return model, nil
			}
		}

	}

	newModel, cmd := model.Model.Update(msg)
	model.Model = &newModel
	cmds = append(cmds, cmd)

	return model, tea.Batch(cmds...)
}
