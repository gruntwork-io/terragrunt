package list

import (
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
	quitFn   func(error)
}

func NewModel(modules module.Modules, quitFn func(error)) *Model {
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
		quitFn:   quitFn,
	}
}

func (model Model) Init() tea.Cmd {
	return nil
}

func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		topPadding := 1
		rightPadding := 2
		h, v := lipgloss.NewStyle().Padding(topPadding, rightPadding).GetFrameSize()
		model.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if model.FilterState() == list.Filtering {
			break
		}

		if key.Matches(msg, model.delegate.Choose, model.delegate.Scaffold) {
			if module, ok := model.SelectedItem().(*module.Module); ok {
				pageModel, err := page.NewModel(module, model.Width(), model.Height(), model, model.quitFn)
				if err != nil {
					model.quitFn(err)
				}

				if key.Matches(msg, model.delegate.Scaffold) {
					if btn := pageModel.Buttons.GetByName(page.ScaffoldButtonName); btn != nil {
						cmd := btn.Action(msg)
						return model, cmd
					}
				}

				return pageModel, nil
			}
		}
	}

	newModel, cmd := model.Model.Update(msg)
	model.Model = &newModel
	cmds = append(cmds, cmd)

	return model, tea.Batch(cmds...)
}
