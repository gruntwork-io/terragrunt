package page

// An example program demonstrating the pager component from the Bubbles
// component library.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
)

const (
	defaultFocusIndex = 1

	buttonScaffoldName      = "Scaffold"
	buttonViewInBrowserName = "View in Browser"
)

var (
	infoPositionStyle = lipgloss.NewStyle().Padding(0, 1).BorderStyle(lipgloss.HiddenBorder())
	infoLineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#1D252"))
)

type model struct {
	viewport      *viewport.Model
	previousModel tea.Model

	height int

	keys       KeyMap
	buttons    Buttons
	focusIndex int
}

func (model model) Init() tea.Cmd {
	return nil
}

func (model model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	cmd = model.keys.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, model.keys.Navigation):
			model.focusIndex++

			maxIndex := model.buttons.Len()

			if model.focusIndex > maxIndex {
				model.focusIndex = 1
			} else if model.focusIndex < 0 {
				model.focusIndex = maxIndex
			}

			model.buttons.Focus(model.focusIndex)

			return model, tea.Batch(cmds...)

		case key.Matches(msg, model.keys.Choose):
			if btn := model.buttons.Get(model.focusIndex); btn != nil {
				cmd := btn.action(msg)
				cmds = append(cmds, cmd)
			}

		case key.Matches(msg, model.keys.Quit):
			return model.previousModel, nil
		case key.Matches(msg, model.keys.ForceQuit):
			return model, tea.Quit
		}

	case tea.WindowSizeMsg:
		model.height = msg.Height
		model.viewport.Width = msg.Width
		model.viewport.Height = msg.Height - lipgloss.Height(model.footerView())
	}

	var viewport viewport.Model
	viewport, cmd = model.viewport.Update(msg)

	model.viewport = &viewport
	cmds = append(cmds, cmd)

	return model, tea.Batch(cmds...)
}

func (model model) View() string {
	footer := model.footerView()
	footerHeight := lipgloss.Height(model.footerView())
	model.viewport.Height = model.height - footerHeight

	return lipgloss.JoinVertical(lipgloss.Left, model.viewport.View(), footer)
}

func (model model) footerView() string {
	info := infoPositionStyle.Render(fmt.Sprintf("%2.f%%", model.viewport.ScrollPercent()*100))

	line := strings.Repeat("─", max(0, model.viewport.Width-lipgloss.Width(info)))
	line = infoLineStyle.Render(line)

	info = lipgloss.JoinHorizontal(lipgloss.Center, line, info)

	return lipgloss.JoinVertical(lipgloss.Left, info, model.buttons.View(), model.keys.View())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func NewModel(module *module.Item, width, height int, previousModel tea.Model) (*model, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	content, err := renderer.Render(module.Readme())
	if err != nil {
		return nil, err
	}

	keys := newKeyMap()

	viewport := viewport.New(width, height)
	viewport.SetContent(string(content))
	viewport.KeyMap = keys.KeyMap

	return &model{
		viewport:      &viewport,
		height:        height,
		keys:          keys,
		previousModel: previousModel,
		focusIndex:    defaultFocusIndex,
		buttons: NewButtons(
			NewButton(buttonScaffoldName, func(msg tea.Msg) tea.Cmd {
				return tea.Exec(module.ScaffoldCommand(), nil)
			}),

			NewButton(buttonViewInBrowserName, func(msg tea.Msg) tea.Cmd {
				module.ViewInBrowser()
				return nil
			}),
		).Focus(defaultFocusIndex),
	}, nil
}
