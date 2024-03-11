package page

// An example program demonstrating the pager component from the Bubbles
// component library.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/pkg/browser"
)

const (
	defaultFocusIndex = 1

	ScaffoldButtonName      = "Scaffold"
	ViewInBrowserButtonName = "View Source in Browser"
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

func NewModel(module *module.Module, width, height int, previousModel tea.Model, quitFn func(error), opts *options.TerragruntOptions) (*Model, error) {
	var content string

	if module.IsMarkDown() {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return nil, err
		}

		content, err = renderer.Render(module.Content(false))
		if err != nil {
			return nil, err
		}
	} else {
		content = module.Content(true)
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
					return ClearScreen()
				}
				result := tea.Exec(command.NewScaffold(opts, module), quitFn)
				return tea.Sequence(result, ClearScreenCmd())
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

	rawMsg := fmt.Sprintf("%T", msg)
	// handle special case for Exit alt screen
	if rawMsg == "tea.execMsg" {
		defer func() {
			os.Exit(0)
		}()
		return model, tea.Sequence(Cmd(ClearScreenCmd()), tea.Quit)
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

// ClearScreen - explicit clear screen to avoid terminal hanging
func ClearScreen() tea.Msg {
	ansiTerminalReset()
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("stty", "sane")
		_ = cmd.Run()
	}
	if runtime.GOOS == "linux" {
		cmd := exec.Command("reset")
		_ = cmd.Run()
	}
	return tea.Sequence(Cmd(tea.ExitAltScreen()), Cmd(tea.ClearScreen()), Cmd(tea.ClearScrollArea()), tea.Quit)
}

func ansiTerminalReset() {
	// https://www.unix.com/os-x-apple-/279401-means-clearing-scroll-buffer-osx-terminal.html
	fmt.Print("\033c")   // Reset the terminal
	fmt.Print("\033[2J") // Clear the screen
	fmt.Print("\033[3J") // Clear buffer
	fmt.Print("\033[H")  // Move the cursor to the home position
	fmt.Print("\033[0m") // Reset all terminal attributes to their defaults
}

// ClearScreenCmd - command to clear the screen
func ClearScreenCmd() tea.Cmd {
	return ClearScreen
}

// Cmd - wrap a message in a command
func Cmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}
