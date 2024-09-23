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

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/components/buttonbar"
)

func updateList(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.delegateKeys.choose, m.delegateKeys.scaffold):
			if selectedModule, ok := m.list.SelectedItem().(*module.Module); ok {
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

					// advance state
					m.selectedModule = selectedModule
					m.state = pagerState
				case key.Matches(msg, m.delegateKeys.scaffold):
					m.state = scaffoldState
					return m, scaffoldModuleCmd(m, selectedModule)
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
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func updatePager(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		bb, cmd := m.buttonBar.Update(msg)
		m.buttonBar = bb.(*buttonbar.ButtonBar)

		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		switch {
		case key.Matches(msg, m.pagerKeys.Choose):
			// Choose changes the action depending on the active button
			if m.activeButton == scaffoldBtn {
				m.state = scaffoldState
				return m, scaffoldModuleCmd(m, m.selectedModule)
			} else {
				if err := browser.OpenURL(m.selectedModule.URL()); err != nil {
					m.viewport.SetContent(fmt.Sprintf("could not open url in browser: %s. got error: %s", m.releaseNotesURL, err))
				}
			}

		case key.Matches(msg, m.pagerKeys.Scaffold):
			m.state = scaffoldState
			return m, scaffoldModuleCmd(m, m.selectedModule)

		case key.Matches(msg, m.pagerKeys.Quit):
			// because we're on the second screen, we need to go back
			m.state = listState
			return m, nil
		}
	case buttonbar.ActiveBtnMsg:
		m.activeButton = button(msg)
	}

	// Handle keyboard and mouse events in the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// Update handles all TUI interactions and implements bubbletea.Model.Update.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
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
		m.state = pagerState
	}

	// Hand off the message and model to the appropriate update function for the
	// appropriate view based on the current state.
	switch m.state {
	case listState:
		return updateList(msg, m)
	case pagerState:
		return updatePager(msg, m)
	case scaffoldState:
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
func scaffoldModuleCmd(m model, module *module.Module) tea.Cmd {
	return tea.Exec(command.NewScaffold(m.terragruntOptions, module), func(err error) tea.Msg {
		return scaffoldFinishedMsg{err}
	})
}
