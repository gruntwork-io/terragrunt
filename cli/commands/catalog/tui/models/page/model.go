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
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/pkg/browser"
)

const (
	defaultFocusIndex = 1

	ScaffoldButtonName      = "Scaffold"
	ViewInBrowserButtonName = "View in Browser"
)

var (
	infoPositionStyle = lipgloss.NewStyle().Padding(0, 1).BorderStyle(lipgloss.HiddenBorder())
	infoLineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#1D252"))
)

type Model struct {
	Buttons Buttons

	viewport      *viewport.Model
	previousModel tea.Model

	height int

	keys       KeyMap
	focusIndex int
}

func NewModel(module *module.Module, width, height int, previousModel tea.Model, quitFn func(error)) (*Model, error) {
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
	viewport.SetContent(content)
	viewport.KeyMap = keys.KeyMap

	return &Model{
		viewport:      &viewport,
		height:        height,
		keys:          keys,
		previousModel: previousModel,
		focusIndex:    defaultFocusIndex,
		Buttons: NewButtons(
			NewButton(ScaffoldButtonName, func(msg tea.Msg) tea.Cmd {
				quitFn := func(err error) tea.Msg {
					quitFn(err)
					return nil
				}
				return tea.Exec(command.NewScaffold(module.Path()), quitFn)
			}),
			NewButton(ViewInBrowserButtonName, func(msg tea.Msg) tea.Cmd {
				if err := browser.OpenURL(module.URL()); err != nil {
					quitFn(err)
				}
				return nil
			}),
		).Focus(defaultFocusIndex),
	}, nil
}

// Init implements bubbletea.Model.Init
func (model Model) Init() tea.Cmd {
	return nil
}

// Update implements bubbletea.Model.Update
func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

			maxIndex := model.Buttons.Len()

			if model.focusIndex > maxIndex {
				model.focusIndex = 1
			} else if model.focusIndex < 0 {
				model.focusIndex = maxIndex
			}

			model.Buttons.Focus(model.focusIndex)

			return model, tea.Batch(cmds...)

		case key.Matches(msg, model.keys.Choose):
			if btn := model.Buttons.Get(model.focusIndex); btn != nil {
				cmd := btn.action(msg)
				return model, cmd
			}

		case key.Matches(msg, model.keys.Scaffold):
			if btn := model.Buttons.GetByName(ScaffoldButtonName); btn != nil {
				cmd := btn.action(msg)
				return model, cmd
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

// View implements bubbletea.Model.View
func (model Model) View() string {
	footer := model.footerView()
	footerHeight := lipgloss.Height(model.footerView())
	model.viewport.Height = model.height - footerHeight

	return lipgloss.JoinVertical(lipgloss.Left, model.viewport.View(), footer)
}

func (model Model) footerView() string {
	var percent float64 = 100
	info := infoPositionStyle.Render(fmt.Sprintf("%2.f%%", model.viewport.ScrollPercent()*percent))

	line := strings.Repeat("─", max(0, model.viewport.Width-lipgloss.Width(info)))
	line = infoLineStyle.Render(line)

	info = lipgloss.JoinHorizontal(lipgloss.Center, line, info)

	return lipgloss.JoinVertical(lipgloss.Left, info, model.Buttons.View(), model.keys.View())
}
