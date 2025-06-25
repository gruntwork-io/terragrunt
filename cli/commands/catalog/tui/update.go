package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func updateList(msg tea.Msg, m Model) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.List.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.delegateKeys.choose, m.delegateKeys.scaffold):
			if selectedModule, ok := m.List.SelectedItem().(*module.Module); ok {
				switch {
				case key.Matches(msg, m.delegateKeys.choose):
					// prepare the viewport
					var content string

					if selectedModule.IsMarkDown() {
						renderer, err := glamour.NewTermRenderer(
							glamour.WithAutoStyle(),
							glamour.WithWordWrap(m.width),
						)
						if err != nil {
							return m, rendererErrCmd(err)
						}

						md, err := renderer.Render(selectedModule.Content(false))
						if err != nil {
							return m, rendererErrCmd(err)
						}

						content = md
					} else {
						content = selectedModule.Content(true)
					}

					m.viewport.SetContent(content)

					// Dynamically create button bar based on module URL
					var pagerButtons []button

					buttonNames := []string{}

					// Always add scaffold button
					pagerButtons = append(pagerButtons, scaffoldBtn)
					buttonNames = append(buttonNames, scaffoldBtn.String())

					if selectedModule.URL() != "" {
						pagerButtons = append(pagerButtons, viewSourceBtn)
						buttonNames = append(buttonNames, viewSourceBtn.String())
					}

					m.currentPagerButtons = pagerButtons
					m.buttonBar = buttonbar.New(buttonNames)
					// Ensure the button bar is initialized
					cmds = append(cmds, m.buttonBar.Init())

					// advance state
					m.selectedModule = selectedModule
					m.State = PagerState
				case key.Matches(msg, m.delegateKeys.scaffold):
					m.State = ScaffoldState
					return m, scaffoldModuleCmd(m.logger, m, m.SVC, selectedModule)
				}
			} else {
				break
			}

		case key.Matches(msg, m.listKeys.Quit):
			// because we're on the first screen, we simply quit at this point
			return m, tea.Quit
		}
	}

	// Handle keyboard and mouse events for the list
	m.List, cmd = m.List.Update(msg)

	// Append any commands from button bar initialization
	if len(cmds) > 0 {
		return m, tea.Batch(cmd, tea.Batch(cmds...))
	}

	return m, cmd
}

func updatePager(msg tea.Msg, m Model) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		bbModel, barCmd := m.buttonBar.Update(msg)
		if newButtonBar, ok := bbModel.(*buttonbar.ButtonBar); ok {
			m.buttonBar = newButtonBar
		}

		if barCmd != nil {
			cmds = append(cmds, barCmd)
		}

		switch {
		case key.Matches(msg, m.pagerKeys.Choose):
			// Choose changes the action depending on the active button
			// m.activeButton is set by ActiveBtnMsg, which is mapped from m.currentPagerButtons
			currentAction := m.activeButton

			switch currentAction {
			case scaffoldBtn:
				m.State = ScaffoldState
				return m, scaffoldModuleCmd(m.logger, m, m.SVC, m.selectedModule)
			case viewSourceBtn:
				if m.selectedModule.URL() != "" {
					if err := browser.OpenURL(m.selectedModule.URL()); err != nil {
						m.viewport.SetContent(fmt.Sprintf("could not open url in browser: %s. got error: %s", m.selectedModule.URL(), err))
					}
				}
			default:
				m.logger.Warnf("Unknown button pressed: %s", currentAction)
			}

		case key.Matches(msg, m.pagerKeys.Scaffold):
			m.State = ScaffoldState
			return m, scaffoldModuleCmd(m.logger, m, m.SVC, m.selectedModule)

		case key.Matches(msg, m.pagerKeys.Quit):
			// because we're on the second screen, we need to go back
			m.State = ListState
			return m, nil
		}
	case buttonbar.ActiveBtnMsg:
		// Map the index from buttonbar.ActiveBtnMsg to the actual button type
		if int(msg) >= 0 && int(msg) < len(m.currentPagerButtons) {
			m.activeButton = m.currentPagerButtons[int(msg)]
		}
	}

	// Handle keyboard and mouse events in the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// Update handles all TUI interactions and implements bubbletea.Model.Update.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.List.SetSize(msg.Width-h, msg.Height-v)
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			m.viewport = viewport.New(msg.Width, msg.Height-v-lipgloss.Height(m.footerView()))
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - v - lipgloss.Height(m.footerView())
		}

	case scaffoldFinishedMsg:
		if msg.err != nil {
			tea.Printf("error scaffolding module: %s", msg.err.Error())
		}

		return m, tea.Quit

	case rendererErrMsg:
		m.viewport.SetContent("there was an error rendering markdown: " + msg.err.Error())
		// ensure we show the viewport
		m.State = PagerState
	}

	// Hand off the message and model to the appropriate update function for the
	// appropriate view based on the current state.
	switch m.State {
	case ListState:
		return updateList(msg, m)
	case PagerState:
		return updatePager(msg, m)
	case ScaffoldState:
		// if we're on the scaffold state, we do nothing and wait for the
		// scaffoldFinishedMsg message. This prevents further input.
		return m, nil
	}

	return m, nil
}

type rendererErrMsg struct{ err error }

func rendererErrCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return rendererErrMsg{err}
	}
}

type scaffoldFinishedMsg struct{ err error }

// Return a tea.Cmd that will scaffold the given module.
func scaffoldModuleCmd(l log.Logger, m Model, svc catalog.CatalogService, module *module.Module) tea.Cmd {
	return tea.Exec(command.NewScaffold(l, m.terragruntOptions, svc, module), func(err error) tea.Msg {
		return scaffoldFinishedMsg{err}
	})
}
